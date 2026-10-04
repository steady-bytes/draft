// Command post-receive-hook is installed as every bare repository's hooks/post-receive (see
// store.go's installHook) -- not run directly; the same generated wrapper-script mechanism as
// pre-receive-hook execs this binary with ALLELE_REPOSITORY_ID/ALLELE_SERVER_ADDR already set.
//
// git invokes post-receive once per push, *after* every ref has been updated and every object
// committed to the real object store -- unlike pre-receive, this hook's own exit code is ignored by
// git (a push can no longer be rejected at this point), and the pushed objects are now visible to a
// plain `git diff` run from Allele's own long-lived server process, not just this hook's own
// quarantined environment. That's what makes it safe for NotifyPush to actually diff -- it does the
// real work server-side (reusing computeChange; see worktree_store.go's own NotifyPush); this binary
// is a thin relay of "this ref moved," not a second diffing engine.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	allelev1 "github.com/steady-bytes/draft/api/tooling/allele/v1"

	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	repositoryID := os.Getenv("ALLELE_REPOSITORY_ID")
	serverAddr := os.Getenv("ALLELE_SERVER_ADDR")
	if repositoryID == "" || serverAddr == "" {
		// post-receive can't block a push either way (git ignores this hook's own exit code), so
		// the only consequence of bailing out here is a missed SymbolChanged event -- worth
		// logging, never worth failing loudly the way pre-receive-hook's own equivalent check does.
		fmt.Fprintln(os.Stderr, "allele post-receive-hook: ALLELE_REPOSITORY_ID/ALLELE_SERVER_ADDR not set; skipping")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		_, newSha, ref := fields[0], fields[1], fields[2]
		if isZeroSha(newSha) {
			continue // ref deletion -- nothing to index
		}
		if err := notifyPush(client, serverAddr, repositoryID, ref, newSha); err != nil {
			// Best-effort, matching this event path's own "observability side-channel" posture
			// (see events.go's own doc comment) -- a failed notification never affects the push,
			// which already succeeded by the time this hook runs.
			fmt.Fprintf(os.Stderr, "allele post-receive-hook: notify failed for %s: %v\n", ref, err)
		}
	}
}

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

// notifyPush calls AlleleService.NotifyPush over Connect's plain JSON+HTTP protocol -- same
// curl-able wire format pre-receive-hook's own checkPushPermission uses, for the same reason (no
// need for a generated Connect client in a binary this small).
func notifyPush(client *http.Client, serverAddr, repositoryID, ref, newCommitSha string) error {
	reqMsg := &allelev1.NotifyPushRequest{
		RepositoryId: repositoryID,
		Ref:          ref,
		NewCommitSha: newCommitSha,
	}
	body, err := protojson.Marshal(reqMsg)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	url := strings.TrimRight(serverAddr, "/") + "/tooling.allele.v1.AlleleService/NotifyPush"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("calling allele server: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("allele server returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}
