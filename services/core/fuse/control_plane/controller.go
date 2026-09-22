package control_plane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	ntv1 "github.com/steady-bytes/draft/api/core/control_plane/networking/v1"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	"github.com/steady-bytes/draft/pkg/chassis"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// controlPlane owns route validation, conflict detection, and persistence
// to Blueprint's key/value store -- identical regardless of which
// ProxyBackend is active. Turning a validated route table into live
// traffic handling is the one thing that differs by backend; see apply()
// and backend.go's ProxyBackend interface.
type controlPlane struct {
	logger  chassis.Logger
	backend ProxyBackend
}

var (
	ErrFailedRouteMarshal = errors.New("failed to marshal route")
	ErrUnableToSaveRoute  = errors.New("unable to save route in the key/value store")
)

// NewControlPlane constructs the backend-agnostic control plane. backend is
// whichever ProxyBackend main.go selected via ProxyBackendName().
func NewControlPlane(logger chassis.Logger, backend ProxyBackend) *controlPlane {
	return &controlPlane{
		logger:  logger,
		backend: backend,
	}
}

func (cp *controlPlane) LoadCache() {
	var (
		ctx    = context.Background()
		client = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	)

	err := cp.apply(ctx, client)
	if err != nil {
		cp.logger.WithError(err).Error("failed to load cache")
	}
}

func (cp *controlPlane) UpdateCacheWithNewRoute(route *ntv1.Route) error {
	var (
		ctx    = context.Background()
		logger = cp.logger.WithField("route_name", route.Name)
		client = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())
	)

	logger.Info("updating cache with new route")

	// upsert route in the blueprint key/value store
	val, err := anypb.New(route)
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrUnableToSaveRoute
	}

	setReq := connect.NewRequest(&kvv1.SetRequest{
		Key:   storageKey(route.GetName(), route.GetEndpoint()),
		Value: val,
	})

	_, err = client.Set(ctx, setReq)
	if err != nil {
		logger.Error(err.Error())
		return ErrUnableToSaveRoute
	}

	return cp.apply(ctx, client)
}

// DeleteRoute removes a route from the blueprint key/value store and rebuilds the Envoy snapshot
// without it, mirroring what UpdateCacheWithNewRoute already does on add. A logical route name can
// be backed by more than one stored registration (eg. every raft node in a Blueprint cluster
// registering the same route -- see storageKey), so this removes every registration sharing name,
// not just one, matching the UI's "delete this whole route" expectation.
func (cp *controlPlane) DeleteRoute(ctx context.Context, name string) error {
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())

	raw, err := cp.listRawRoutes(ctx, client)
	if err != nil {
		return err
	}

	routeModel, err := anypb.New(&ntv1.Route{})
	if err != nil {
		cp.logger.Error(err.Error())
		return ErrFailedRouteMarshal
	}

	for key, r := range raw {
		if r.GetName() != name {
			continue
		}
		if _, err := client.Delete(ctx, connect.NewRequest(&kvv1.DeleteRequest{
			Key:   key,
			Value: routeModel,
		})); err != nil {
			cp.logger.Error(err.Error())
			return err
		}
	}

	return cp.apply(ctx, client)
}

// FindConflicts returns the names of any existing routes that share the same (host, match_type,
// prefix) tuple as candidate. Excludes candidate.Name itself so re-registering an unchanged route
// doesn't flag against itself, and excludes existingName (when non-empty) so validating a rename
// — candidate.Name is the new name, existingName the route's name before the edit — doesn't flag
// against its own not-yet-deleted prior version. AddRoute's own call passes "" (no additional
// exclusion; by the time it runs during a rename, the old name has already been deleted).
func (cp *controlPlane) FindConflicts(ctx context.Context, candidate *ntv1.Route, existingName string) ([]string, error) {
	existing, err := cp.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	key := routeKey(candidate.GetMatch())
	var conflicts []string
	for _, r := range existing {
		if r.GetName() == candidate.GetName() || (existingName != "" && r.GetName() == existingName) {
			continue
		}
		if r.GetMatch().GetHost() == candidate.GetMatch().GetHost() && routeKey(r.GetMatch()) == key {
			conflicts = append(conflicts, r.GetName())
		}
	}
	return conflicts, nil
}

// checkCapabilities returns a non-empty rejection message if candidate
// requires something the active backend's Capabilities() says it can't
// honor -- Phase 8's config-time validation, so a setting the backend
// can't enforce is rejected at registration instead of silently accepted
// and never actually applied.
//
// mTLS is the only thing checked here on purpose. WideEvents
// (wide_events_disabled) degrades safely when unsupported -- a backend
// that can't produce one just doesn't, same as an ordinary opt-out — so
// there's nothing to reject; see the design doc's WideEvent section.
// TLS has no per-route field to check at all: it's a listener-level
// setting (fuse.tls.mode), not something an individual Route opts into.
// mTLS is different in kind: silently not enforcing a client-certificate
// requirement is a real security downgrade, not a graceful no-op, which is
// exactly the gap this phase exists to close.
func (cp *controlPlane) checkCapabilities(candidate *ntv1.Route) string {
	caps := cp.backend.Capabilities()
	if candidate.GetMtls().GetEnabled() && !caps.MTLS {
		return fmt.Sprintf("route requires mTLS, but the active proxy backend (%s) doesn't support it", cp.backend.Name())
	}
	return ""
}

// routeKey normalizes match_type (UNSPECIFIED behaves as PREFIX, matching the compiled Envoy
// behavior in makeRouterConfig) so two routes that would compile to the same Envoy route conflict
// even if one left match_type unset.
func routeKey(m *ntv1.RouteMatch) string {
	mt := m.GetMatchType()
	if mt == ntv1.MatchType_MATCH_TYPE_UNSPECIFIED {
		mt = ntv1.MatchType_MATCH_TYPE_PREFIX
	}
	return fmt.Sprintf("%d:%s", mt, m.GetPrefix())
}

// ListRoutes returns one logical route per registered name, merging every stored registration
// that shares a name (see storageKey) into a single Route with Endpoints populated from all of
// them. This is the view every caller outside this file should use -- conflict detection, the
// RPC handler, and the gateway UI all want "one row per route", not one row per backend instance.
func (cp *controlPlane) ListRoutes(ctx context.Context) ([]*ntv1.Route, error) {
	client := kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, chassis.GetConfig().Entrypoint())

	raw, err := cp.listRawRoutes(ctx, client)
	if err != nil {
		return nil, err
	}
	return mergeRoutes(raw), nil
}

// listRawRoutes returns every stored route registration keyed by its raw KV key (one entry per
// storageKey, so a load-balanced route with N registered backends has N entries here, all sharing
// the same Route.Name). Callers that need "one row per logical route" should go through
// ListRoutes/mergeRoutes instead; this is for the two places that need the raw per-registration
// keys themselves: DeleteRoute (to remove every registration under a name) and apply (which merges
// them into Envoy clusters directly, skipping ListRoutes' extra Route re-serialization).
func (cp *controlPlane) listRawRoutes(ctx context.Context, client kvv1Connect.KeyValueServiceClient) (map[string]*ntv1.Route, error) {
	routeModel, err := anypb.New(&ntv1.Route{})
	if err != nil {
		return nil, ErrFailedRouteMarshal
	}

	resp, err := client.List(ctx, connect.NewRequest(&kvv1.ListRequest{Value: routeModel}))
	if err != nil {
		return nil, ErrUnableToSaveRoute
	}

	raw := make(map[string]*ntv1.Route, len(resp.Msg.GetValues()))
	for key, v := range resp.Msg.GetValues() {
		r := &ntv1.Route{}
		if err := v.UnmarshalTo(r); err != nil {
			return nil, ErrFailedRouteMarshal
		}
		// List returns each entry keyed by its physical storage key ("<type_url>-<key>", see
		// blueprint's key_value.Model.makeKey), not the logical key Set/Get/Delete take -- those
		// three re-apply the type_url prefix themselves. Passing a physical key straight back into
		// Delete would double-prefix it and silently delete nothing (found live: a second
		// registration's endpoint survived a DeleteRoute call that reported OK). Strip it back to
		// the logical key here, the same way ListKinds already does, so callers of this map (namely
		// DeleteRoute) can pass a key straight to Delete. Type_urls never contain a hyphen, so
		// splitting on the first one unambiguously recovers the logical key.
		_, logicalKey, found := strings.Cut(key, "-")
		if !found {
			continue
		}
		raw[logicalKey] = r
	}
	return raw, nil
}

// mergeRoutes groups raw per-registration routes by Name into one logical route per name.
// Multiple processes registering the identical (name, match) tuple -- eg. every raft node in a
// Blueprint cluster registering the same UI route -- are meant to be load-balanced as one Envoy
// cluster, not treated as separate routes or clobber each other. The first registration in sorted
// key order wins for every non-endpoint field (match, auth, http2); since routes sharing a name
// are expected to share a definition, this is only a real choice for the endpoint fields, where
// Endpoint keeps that same first instance (for existing single-endpoint readers) and Endpoints
// carries the full deduplicated set. Sorting keys first (rather than ranging the map directly)
// keeps that "first" choice deterministic across repeated calls instead of flapping with Go's
// randomized map iteration order.
func mergeRoutes(raw map[string]*ntv1.Route) []*ntv1.Route {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	byName := make(map[string]*ntv1.Route, len(raw))
	seenEndpoint := make(map[string]map[string]bool, len(raw))
	var order []string

	for _, k := range keys {
		r := raw[k]
		name := r.GetName()
		merged, ok := byName[name]
		if !ok {
			merged = proto.Clone(r).(*ntv1.Route)
			merged.Endpoints = nil
			byName[name] = merged
			seenEndpoint[name] = map[string]bool{}
			order = append(order, name)
		}
		if ep := r.GetEndpoint(); ep != nil {
			ek := endpointKey(ep)
			if !seenEndpoint[name][ek] {
				seenEndpoint[name][ek] = true
				merged.Endpoints = append(merged.Endpoints, ep)
			}
		}
	}

	routes := make([]*ntv1.Route, 0, len(order))
	for _, name := range order {
		routes = append(routes, byName[name])
	}
	return routes
}

// storageKey is the KV key a single route registration is stored under. Composed from the route's
// name and its own endpoint (rather than just the name) so that multiple processes registering
// the identical route name -- the normal case for a horizontally-scaled or raft-clustered service,
// where every instance calls chassis's WithRoute with the same Route.Name and its own address --
// each get their own KV entry instead of overwriting each other's. mergeRoutes groups these back
// into one logical route per name when reading. A process re-registering from the same address
// (eg. across a restart) reuses the same key, so that registration still upserts in place rather
// than accumulating.
func storageKey(name string, e *ntv1.Endpoint) string {
	return fmt.Sprintf("%s@%s", name, endpointKey(e))
}

func endpointKey(e *ntv1.Endpoint) string {
	return fmt.Sprintf("%s:%d", e.GetHost(), e.GetPort())
}

// apply merges the currently-persisted routes and hands them to whichever
// ProxyBackend is active. All the backend-specific work (Envoy snapshot
// building, or the native backend's in-memory table swap once it exists)
// lives on the backend's own Apply method -- see backend.go and
// envoy_backend.go.
func (cp *controlPlane) apply(ctx context.Context, client kvv1Connect.KeyValueServiceClient) error {
	raw, err := cp.listRawRoutes(ctx, client)
	if err != nil {
		return err
	}
	// Merge every raw registration into one logical route per name before
	// handing it to the backend -- each becomes exactly one cluster/route
	// (or native-backend table entry), instead of one per raw registration.
	return cp.backend.Apply(mergeRoutes(raw))
}
