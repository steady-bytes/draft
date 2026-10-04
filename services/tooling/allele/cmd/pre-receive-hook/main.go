// Command pre-receive-hook is installed as every bare repository's hooks/pre-receive (see
// store.go's installHook) -- not run directly; a small generated wrapper script execs this binary
// with ALLELE_REPOSITORY_ID/ALLELE_SERVER_ADDR already set in its environment.
//
// git invokes pre-receive once per push, before any ref is updated, with one line per ref on stdin
// ("<old-sha> <new-sha> <ref-name>") and the whole push's environment inherited -- including
// HTTP_X_AUTHENTIK_USERNAME (confirmed this session to propagate correctly through
// net/http/cgi.Handler -> git-receive-pack -> this hook) and git's own quarantine variables
// (GIT_QUARANTINE_PATH/GIT_OBJECT_DIRECTORY), which make the pushed-but-not-yet-accepted objects
// visible to a plain `git diff` run from here. A non-zero exit rejects the *entire* push (git's own
// pre-receive semantics are all-or-nothing across every ref line, never partial), so every line is
// checked before deciding, and every violation found is reported at once rather than just the first.
//
// R4.2's enforcement; R4.1's attribution is recorded server-side as a side effect of an allowed
// check (see rpc.go's CheckPushPermission) -- this binary itself persists nothing.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"

	"google.golang.org/protobuf/encoding/protojson"
)

// defaultBranch matches store.go's CreateRepository, which always runs `git init
// --bare --initial-branch=main` -- there is no RPC to change a Repository's DefaultBranch after
// creation (its field exists for display, not configuration), so hardcoding it here to find a
// brand-new branch's diff base is a disclosed simplification, not a guess.
const defaultBranch = "main"

func main() {
	repositoryID := os.Getenv("ALLELE_REPOSITORY_ID")
	serverAddr := os.Getenv("ALLELE_SERVER_ADDR")
	if repositoryID == "" || serverAddr == "" {
		// Fail closed: a hook that can't identify itself or reach the permission service must
		// never let a push through silently.
		fmt.Fprintln(os.Stderr, "allele pre-receive-hook: ALLELE_REPOSITORY_ID/ALLELE_SERVER_ADDR not set; rejecting push")
		os.Exit(1)
	}

	repoPath, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "allele pre-receive-hook: resolving working directory: %v\n", err)
		os.Exit(1)
	}

	identity := os.Getenv("HTTP_X_AUTHENTIK_USERNAME")

	client := &http.Client{Timeout: 10 * time.Second}
	var violations []string

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		oldSha, newSha, ref := fields[0], fields[1], fields[2]
		if isZeroSha(newSha) {
			continue // ref deletion -- nothing pushed, nothing to check
		}

		base := oldSha
		if isZeroSha(oldSha) {
			// Brand-new branch: nothing came "before" it on this ref, so diff from where it
			// actually forked off the default branch instead -- gitutil.ChangedFiles's own
			// "..." convention, reimplemented here rather than imported since this binary
			// intentionally stays dependency-light (it runs on every single push).
			mergeBase, err := runGit(repoPath, "merge-base", defaultBranch, newSha)
			if err != nil {
				// The very first push to a brand-new repo (nothing to merge-base against yet)
				// has no default branch to compare to -- every file in newSha is, by
				// definition, new, so diff against git's own empty-tree sentinel instead.
				mergeBase = emptyTreeSha
			}
			// runGit returns raw stdout, trailing newline included (same contract as
			// gitutil.output -- each caller trims for itself); found live on the very first
			// real push through this hook, where the untrimmed value spliced into "base..compare"
			// below produced a single argument with an embedded newline and a bogus
			// "ambiguous argument" failure from `git diff` instead of a real path list.
			base = strings.TrimSpace(mergeBase)
		}

		paths, err := changedPaths(repoPath, base, newSha)
		if err != nil {
			fmt.Fprintf(os.Stderr, "allele pre-receive-hook: computing changed paths for %s: %v\n", ref, err)
			violations = append(violations, fmt.Sprintf("%s: could not compute changed paths", ref))
			continue
		}
		if len(paths) == 0 {
			continue
		}

		allowed, reason, err := checkPushPermission(client, serverAddr, repositoryID, identity, ref, newSha, paths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "allele pre-receive-hook: permission check failed for %s: %v\n", ref, err)
			violations = append(violations, fmt.Sprintf("%s: permission service unreachable", ref))
			continue
		}
		if !allowed {
			violations = append(violations, fmt.Sprintf("%s: %s", ref, reason))
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "allele pre-receive-hook: reading ref updates: %v\n", err)
		os.Exit(1)
	}

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "allele: push rejected -- path permission violations:")
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "  - %s\n", v)
		}
		os.Exit(1)
	}
}

// emptyTreeSha is git's well-known hash of the empty tree -- the same constant `git diff` itself
// uses internally when diffing "nothing" against a real commit; valid in every git repository
// without needing to exist as an actual object first.
const emptyTreeSha = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func isZeroSha(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

// changedPaths shells out directly (rather than importing the gitutil package) so this binary pulls
// in as little as possible beyond the standard library and the one generated proto package it needs
// for the wire format -- it runs synchronously on every push, and its own dependency surface is
// worth keeping small and auditable independent of the rest of the service.
func changedPaths(repoPath, base, compare string) ([]string, error) {
	out, err := runGit(repoPath, "diff", "--name-only", base+".."+compare)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// runGit inherits this process's own environment (cmd.Env left nil) deliberately -- git's
// quarantine variables (GIT_QUARANTINE_PATH/GIT_OBJECT_DIRECTORY), set by git itself before
// invoking pre-receive, are what make a brand-new push's not-yet-accepted objects visible to this
// command at all; a child process that didn't inherit them would see the push's new commits as
// missing objects.
func runGit(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// checkPushPermission calls AlleleService.CheckPushPermission over Connect's plain JSON+HTTP
// protocol -- the same curl-able wire format used throughout this project's own live verification,
// not a generated Connect client, since this binary has no need for the rest of that client's
// surface and stays simpler shelling out to one well-known URL instead.
func checkPushPermission(client *http.Client, serverAddr, repositoryID, identity, ref, newCommitSha string, paths []string) (allowed bool, reason string, err error) {
	reqMsg := &allelev1.CheckPushPermissionRequest{
		RepositoryId: repositoryID,
		Identity:     identity,
		Paths:        paths,
		Ref:          ref,
		NewCommitSha: newCommitSha,
	}
	body, err := protojson.Marshal(reqMsg)
	if err != nil {
		return false, "", fmt.Errorf("marshaling request: %w", err)
	}

	url := strings.TrimRight(serverAddr, "/") + "/tooling.allele.v1.AlleleService/CheckPushPermission"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, "", fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return false, "", fmt.Errorf("calling allele server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("allele server returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}

	var respMsg allelev1.CheckPushPermissionResponse
	if err := protojson.Unmarshal(respBody, &respMsg); err != nil {
		return false, "", fmt.Errorf("unmarshaling response: %w", err)
	}
	return respMsg.GetAllowed(), respMsg.GetReason(), nil
}
