package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"ravenseed/internal/database"
	"ravenseed/internal/models"
)

func schema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				inner := schema(f.Type)
				for name, value := range inner["properties"].(map[string]any) {
					props[name] = value
				}
				required = append(required, inner["required"].([]string)...)
				continue
			}
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			// openapi:"-" marks stored fields a custom MarshalJSON leaves out.
			if name == "-" || f.Tag.Get("openapi") == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			props[name] = schema(f.Type)
			if !strings.Contains(tag, "omitempty") && !strings.Contains(tag, "omitzero") {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": props, "required": required}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schema(t.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schema(t.Elem())}
	default:
		return map[string]any{"type": "string"}
	}
}
func ref(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }

// openAPISpec is built once; the served document never changes.
var openAPISpec = sync.OnceValue(OpenAPI)

func OpenAPI() map[string]any {
	schemas := map[string]any{}
	for name, t := range map[string]reflect.Type{"SearchRequest": reflect.TypeFor[models.SearchRequest](), "Job": reflect.TypeFor[models.Job](), "Family": reflect.TypeFor[models.Family](), "Result": reflect.TypeFor[models.Result](), "Indexer": reflect.TypeFor[models.Indexer](), "Settings": reflect.TypeFor[models.Settings](), "Error": reflect.TypeFor[ErrorBody](), "Event": reflect.TypeFor[database.Event](), "JobsPage": reflect.TypeFor[Page[models.Job]](), "FamiliesPage": reflect.TypeFor[Page[models.Family]](), "ResultsPage": reflect.TypeFor[Page[models.Result]]()} {
		schemas[name] = schema(t)
	}
	paths := map[string]any{}
	schemas["SearchOverrides"] = schema(reflect.TypeFor[map[int]models.IndexerSearchOverride]())
	schemas["MatchingRules"] = schema(reflect.TypeFor[models.MatchingRules]())
	schemas["MetadataSettings"] = schema(reflect.TypeFor[models.MetadataSettings]())
	schemas["ArrSettings"] = schema(reflect.TypeFor[models.ArrSettings]())
	schemas["ArrInstance"] = schema(reflect.TypeFor[models.ArrInstance]())
	schemas["ResolveRequest"] = schema(reflect.TypeFor[models.ResolveRequest]())
	schemas["MediaSnapshot"] = schema(reflect.TypeFor[models.MediaSnapshot]())
	schemas["IndexerPreset"] = schema(reflect.TypeFor[models.IndexerPreset]())
	schemas["Presets"] = schema(reflect.TypeFor[[]models.IndexerPreset]())
	schemas["QuiInstances"] = schema(reflect.TypeFor[[]models.QuiInstance]())
	schemas["QuiCategories"] = schema(reflect.TypeFor[[]string]())
	schemas["UploadSettings"] = schema(reflect.TypeFor[models.UploadSettings]())
	schemas["UploadJobs"] = schema(reflect.TypeFor[UploadJobsPage]())
	schemas["UploadJob"] = schema(reflect.TypeFor[models.UploadJob]())
	schemas["RetryUploadRequest"] = schema(reflect.TypeFor[models.RetryUploadRequest]())
	schemas["CreateMonitorRequest"] = schema(reflect.TypeFor[models.CreateMonitorRequest]())
	schemas["MonitorStateRequest"] = schema(reflect.TypeFor[models.MonitorStateRequest]())
	schemas["MonitorUploadRequest"] = schema(reflect.TypeFor[models.MonitorUploadRequest]())
	schemas["Monitors"] = schema(reflect.TypeFor[[]models.Monitor]())
	schemas["Monitor"] = schema(reflect.TypeFor[models.Monitor]())
	schemas["TrackerHosts"] = schema(reflect.TypeFor[[]models.TrackerHost]())
	schemas["MonitorCandidates"] = schema(reflect.TypeFor[Page[models.MonitorCandidate]]())
	schemas["MonitorCandidate"] = schema(reflect.TypeFor[models.MonitorCandidate]())
	schemas["UploadQueued"] = schema(reflect.TypeFor[models.UploadJob]())
	schemas["IndexerPolicy"] = schema(reflect.TypeFor[models.IndexerPolicy]())
	schemas["IndexerPolicies"] = schema(reflect.TypeFor[[]models.IndexerPolicy]())
	schemas["PolicyPresets"] = schema(reflect.TypeFor[[]models.PolicyPreset]())
	schemas["PolicyEvaluation"] = schema(reflect.TypeFor[models.PolicyEvaluation]())
	schemas["PolicyEvaluationRequest"] = map[string]any{"type": "object", "properties": map[string]any{"searchId": map[string]any{"type": "string"}, "candidateResultId": map[string]any{"type": "string"}, "targetResultIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, "required": []string{"searchId", "candidateResultId"}}
	schemas["PolicyRecheck"] = map[string]any{"type": "object", "properties": map[string]any{"queued": map[string]any{"type": "integer"}}}
	uploadProps := schemas["UploadSettings"].(map[string]any)["properties"].(map[string]any)
	uploadProps["token"].(map[string]any)["writeOnly"] = true
	uploadProps["hasToken"].(map[string]any)["readOnly"] = true
	schemas["AddTorrentRequest"] = schema(reflect.TypeFor[models.AddTorrentRequest]())
	schemas["AddTorrentResponse"] = schema(reflect.TypeFor[models.AddTorrentResponse]())
	schemas["IndexerPreset"].(map[string]any)["required"] = []string{"name", "indexerIds"}
	schemas["AddTorrentRequest"].(map[string]any)["properties"].(map[string]any)["paused"].(map[string]any)["default"] = true
	schemas["Deleted"] = map[string]any{"type": "object", "properties": map[string]any{"deleted": map[string]any{"type": "boolean"}}}
	add := func(path, method, summary, request, response string, paged bool) {
		params := []any{}
		for _, part := range strings.Split(path, "/") {
			if strings.HasPrefix(part, "{") {
				params = append(params, map[string]any{"name": strings.Trim(part, "{}"), "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
			}
		}
		if paged {
			params = append(params, map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}}, map[string]any{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0, "default": 0}})
		}
		content := map[string]any{"application/json": map[string]any{"schema": ref(response)}}
		responses := map[string]any{"200": map[string]any{"description": "Success", "content": content}}
		for _, code := range []string{"400", "401", "403", "404", "409", "413", "415", "429", "500", "502", "504"} {
			responses[code] = map[string]any{"description": "Request failed", "content": map[string]any{"application/json": map[string]any{"schema": ref("Error")}}}
		}
		op := map[string]any{"summary": summary, "parameters": params, "responses": responses, "security": []any{map[string]any{"bearerAuth": []string{}}, map[string]any{"browserSession": []string{}}, map[string]any{"localBrowserSession": []string{}}, map[string]any{}}}
		if request != "" {
			op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": ref(request)}}}
		}
		if path == "/api/v1/searches" && method == "post" {
			responses["202"] = map[string]any{"description": "Search started", "content": content}
		}
		if path == "/api/v1/monitors" && method == "post" {
			responses["201"] = map[string]any{"description": "Monitor created", "content": content}
		}
		if strings.HasPrefix(path, "/api/v1/monitor-candidates/") && (strings.HasSuffix(path, "/check") || strings.HasSuffix(path, "/upload")) && method == "post" {
			responses["202"] = map[string]any{"description": "Check or upload queued", "content": content}
		}
		if strings.HasSuffix(path, "/recheck") && method == "post" {
			responses["202"] = map[string]any{"description": "Fresh monitor checks queued", "content": content}
		}
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		paths[path].(map[string]any)[method] = op
	}
	add("/api/v1/health", "get", "Public process and SQLite health; never contacts upstream services", "", "Health", false)
	schemas["Health"] = map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}, "database": map[string]any{"type": "string"}}}
	paths["/api/v1/health"].(map[string]any)["get"].(map[string]any)["security"] = []any{}
	add("/api/v1/ready", "get", "Rate-limited dependency readiness; requires bearer authentication in token mode", "", "Readiness", false)
	add("/api/v1/session", "get", "Read browser session status", "", "Session", false)
	schemas["Session"] = map[string]any{"type": "object", "properties": map[string]any{"enabled": map[string]any{"type": "boolean"}, "authenticated": map[string]any{"type": "boolean"}}}
	schemas["Readiness"] = map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}, "qui": map[string]any{"type": "string"}, "database": map[string]any{"type": "string"}}}
	for path, response := range map[string]string{"/api/v1/health": "Health", "/api/v1/ready": "Readiness"} {
		paths[path].(map[string]any)["get"].(map[string]any)["responses"].(map[string]any)["503"] = map[string]any{"description": "Service or dependency unavailable", "content": map[string]any{"application/json": map[string]any{"schema": ref(response)}}}
	}
	add("/api/v1/settings", "get", "Read settings without secrets", "", "Settings", false)
	schemas["TorrentSettings"] = schema(reflect.TypeFor[models.TorrentSettings]())
	add("/api/v1/settings/torrents", "get", "Read default qui instance and enabled categories", "", "TorrentSettings", false)
	add("/api/v1/settings/torrents", "put", "Save default qui instance and category selections", "TorrentSettings", "TorrentSettings", false)
	add("/api/v1/settings", "put", "Test and save settings", "Settings", "Settings", false)
	add("/api/v1/settings/search-overrides", "put", "Set movie and TV search methods per indexer (auto, imdb, tmdb, tvdb for TV, title)", "SearchOverrides", "Settings", false)
	add("/api/v1/settings/matching", "put", "Save matching rules without changing the qui connection", "MatchingRules", "Settings", false)
	add("/api/v1/settings/metadata", "put", "Test and save TMDb token and language", "MetadataSettings", "Settings", false)
	add("/api/v1/settings/metadata/test", "post", "Test TMDb without saving", "MetadataSettings", "Connection", false)
	add("/api/v1/settings/uploads", "get", "Read upload settings without secrets", "", "UploadSettings", false)
	add("/api/v1/settings/uploads", "put", "Save Upload-Assistant connection and mappings", "UploadSettings", "UploadSettings", false)
	add("/api/v1/settings/uploads/test", "post", "Test Upload-Assistant API authentication without executing an upload", "UploadSettings", "Connection", false)
	add("/api/v1/upgrades", "get", "List monitor torrents whose slot on the target holds a possibly better release", "", "MonitorCandidates", true)
	add("/api/v1/monitor-candidates/{candidateId}/upgrade/dismiss", "post", "Hide or restore a torrent's upgrade offer; 409 when the offer changed meanwhile", "UpgradeDismissRequest", "MonitorCandidate", false)
	schemas["UpgradeDismissRequest"] = map[string]any{"type": "object", "properties": map[string]any{"dismissed": map[string]any{"type": "boolean"}}, "required": []string{"dismissed"}, "additionalProperties": false}
	add("/api/v1/monitor-candidates/{candidateId}/upgrade/add", "post", "Add the upgrade release from the target to qui; the target is searched again for its link", "AddTorrentRequest", "AddTorrentResponse", false)
	add("/api/v1/settings/arr", "get", "Read Sonarr and Radarr instances without API keys", "", "ArrSettings", false)
	add("/api/v1/settings/arr", "put", "Save Sonarr and Radarr instances; blank keys keep saved keys and cached torrent IDs are cleared", "ArrSettings", "ArrSettings", false)
	add("/api/v1/settings/arr/test", "post", "Test one Sonarr or Radarr instance without saving", "ArrInstance", "Connection", false)
	add("/api/v1/uploads", "get", "List persistent upload jobs, newest first; statusCounts covers the whole queue", "", "UploadJobs", true)
	paths["/api/v1/uploads"].(map[string]any)["get"].(map[string]any)["parameters"] = append(paths["/api/v1/uploads"].(map[string]any)["get"].(map[string]any)["parameters"].([]any), map[string]any{"name": "status", "in": "query", "description": "Comma-separated statuses to list; total counts only these", "schema": map[string]any{"type": "string"}, "example": "adding,downloading,uploading"})
	add("/api/v1/tracker-hosts", "get", "List announce hosts in active qui instances", "", "TrackerHosts", false)
	add("/api/v1/monitors", "get", "List saved tracker pair monitors", "", "Monitors", false)
	add("/api/v1/monitors", "post", "Monitor existing torrents from one tracker against another qui indexer", "CreateMonitorRequest", "Monitor", false)
	add("/api/v1/monitors/{monitorId}", "put", "Pause or resume inventory scans and queued target rechecks, or replace the qui instances scanned; send either field or both. instanceIds [] scans every instance, including ones added later. An instance the monitor already covers is kept while offline; an added one must be active and connected (400 otherwise). Candidates on dropped instances turn gone at their next check; gone ones on re-added instances are checked again. Saving instances clears the last scan error, then an enabled monitor scans the new selection at once; when none of its instances is online, the offline error is set again and the save still succeeds. Fails with 502 when qui cannot be reached", "MonitorStateRequest", "Monitor", false)
	add("/api/v1/monitors/{monitorId}", "delete", "Delete a monitor and its candidate records", "", "Deleted", false)
	add("/api/v1/monitors/{monitorId}/scan", "post", "Refresh the monitor's qui torrent inventory; any request body is ignored. 409 while paused, 400 when the qui connection changed, 502 when qui fails or none of the monitor's instances is active and connected, 504 on a timeout", "", "ScanQueued", false)
	add("/api/v1/monitors/{monitorId}/candidates", "get", "List monitor checks and upload candidates", "", "MonitorCandidates", true)
	add("/api/v1/monitor-candidate-counts", "get", "Count every monitor's candidates per classification filter of the candidates list", "", "MonitorCandidateCounts", false)
	schemas["MonitorCandidateCounts"] = schema(reflect.TypeFor[map[string]map[string]int]())
	schemas["MonitorCandidateCounts"].(map[string]any)["description"] = "Monitor ID to classification filter (" + strings.Join(database.CandidateCountClasses, ", ") + ") to the total that filter lists"
	add("/api/v1/monitor-candidates/{candidateId}/check", "post", "Queue a fresh target search; 409 while paused or already checking", "Empty", "CheckQueued", false)
	add("/api/v1/monitor-candidates/{candidateId}/upload", "post", "Queue an existing candidate torrent for Upload-Assistant; 400 when the monitor no longer scans the torrent's qui instance", "MonitorUploadRequest", "UploadQueued", false)
	schemas["ScanQueued"] = map[string]any{"type": "object", "properties": map[string]any{"scanned": map[string]any{"type": "boolean"}}}
	schemas["CheckQueued"] = map[string]any{"type": "object", "properties": map[string]any{"queued": map[string]any{"type": "boolean"}}}
	add("/api/v1/uploads/{uploadId}/cancel", "post", "Cancel a waiting upload without removing the torrent", "Empty", "Cancelled", false)
	add("/api/v1/uploads/{uploadId}/retry", "post", "Explicitly reupload an inactive job with replacement flags and an optional TMDb ID", "RetryUploadRequest", "UploadJob", false)
	schemas["Empty"] = map[string]any{"type": "object", "additionalProperties": false}
	schemas["Cancelled"] = map[string]any{"type": "object", "properties": map[string]any{"cancelled": map[string]any{"type": "boolean"}}}
	add("/api/v1/media/resolve", "post", "Resolve an IMDb or TMDb ID; warnings preserve direct-ID search", "ResolveRequest", "MediaSnapshot", false)
	add("/api/v1/settings/test", "post", "Test qui connection without saving", "Settings", "Connection", false)
	schemas["Connection"] = map[string]any{"type": "object", "properties": map[string]any{"connected": map[string]any{"type": "boolean"}, "indexers": map[string]any{"type": "array", "items": ref("Indexer")}}}
	add("/api/v1/indexers", "get", "List configured qui indexers", "", "Indexers", false)
	add("/api/v1/indexer-policies", "get", "List saved target indexer policies", "", "IndexerPolicies", false)
	add("/api/v1/policy-presets", "get", "List four generic policy presets derived from supplied rulesets", "", "PolicyPresets", false)
	add("/api/v1/indexer-policies/{indexerId}", "get", "Read a saved policy or disabled default", "", "IndexerPolicy", false)
	add("/api/v1/indexer-policies/{indexerId}", "put", "Save a policy and immutable snapshot", "IndexerPolicy", "IndexerPolicy", false)
	add("/api/v1/indexer-policies/{indexerId}", "delete", "Disable policy by deleting its active configuration", "", "Deleted", false)
	add("/api/v1/indexer-policies/{indexerId}/evaluate", "post", "Evaluate results from a saved search with the active policy", "PolicyEvaluationRequest", "PolicyEvaluation", false)
	add("/api/v1/indexer-policies/{indexerId}/recheck", "post", "Queue fresh monitor target checks for this policy; any request body is ignored", "", "PolicyRecheck", false)
	add("/api/v1/presets", "get", "List saved indexer presets", "", "Presets", false)
	add("/api/v1/presets", "put", "Create or update a preset; omit id to create", "IndexerPreset", "IndexerPreset", false)
	add("/api/v1/presets/{presetId}", "delete", "Delete an indexer preset", "", "Deleted", false)
	add("/api/v1/qui/instances", "get", "List qui torrent destinations", "", "QuiInstances", false)
	add("/api/v1/qui/instances/{instanceId}/categories", "get", "List category names for a qui instance", "", "QuiCategories", false)
	add("/api/v1/searches/{id}/results/{resultId}/add", "post", "Add one saved result to a connected qui instance; paused by default. 400 for a rejected input or a result without a usable download link, 502 when qui fails, 504 when qui does not answer in time (with uploadAfterDownload, check Upload queue before retrying), 500 when the upload job cannot be saved", "AddTorrentRequest", "AddTorrentResponse", false)
	schemas["Indexers"] = map[string]any{"type": "array", "items": ref("Indexer")}
	add("/api/v1/searches", "post", "Start an asynchronous search or return cached analysis", "SearchRequest", "Job", false)
	add("/api/v1/searches", "get", "List search history", "", "JobsPage", true)
	add("/api/v1/searches/{id}", "get", "Read search status and indexer errors", "", "Job", false)
	add("/api/v1/searches/{id}/results", "get", "Read original and parsed results", "", "ResultsPage", true)
	add("/api/v1/searches/{id}/families", "get", "List ranked release families", "", "FamiliesPage", true)
	add("/api/v1/searches/{id}/families/{familyId}", "get", "Read a family with variants and evidence", "", "Family", false)
	add("/api/v1/searches/{id}/candidates", "get", "List target opportunities", "", "FamiliesPage", true)
	add("/api/v1/searches/{id}/events", "get", "Replay durable search events as SSE", "", "Job", false)
	for _, path := range []string{"/api/v1/searches/{id}/families", "/api/v1/searches/{id}/candidates"} {
		op := paths[path].(map[string]any)["get"].(map[string]any)
		p := op["parameters"].([]any)
		for _, name := range []string{"resolution", "releaseType", "source", "hdr", "codec", "group", "confidence"} {
			p = append(p, map[string]any{"name": name, "in": "query", "description": "Comma-separated values", "schema": map[string]any{"type": "string"}})
		}
		p = append(p, map[string]any{"name": "sort", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"coverage", "quality", "seeders", "size", "newest", "target"}}}, map[string]any{"name": "minimumCoverage", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000}})
		p = append(p, map[string]any{"name": "order", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"asc", "desc"}, "default": "desc"}})
		for _, name := range []string{"targetMissing", "exactOnly"} {
			p = append(p, map[string]any{"name": name, "in": "query", "schema": map[string]any{"type": "boolean"}})
		}
		op["parameters"] = p
	}
	query := func(name, description string, schema map[string]any) map[string]any {
		return map[string]any{"name": name, "in": "query", "description": description, "schema": schema}
	}
	text := map[string]any{"type": "string"}
	candidatesOp := paths["/api/v1/monitors/{monitorId}/candidates"].(map[string]any)["get"].(map[string]any)
	candidatesOp["parameters"] = append(candidatesOp["parameters"].([]any),
		query("q", "Name or group; separators, case and accents are ignored", map[string]any{"type": "string", "maxLength": 200}),
		query("sort", "Order by last check", map[string]any{"type": "string", "enum": []string{"checked_desc", "checked_asc"}}),
		query("classification", "all, errors, gone, queued (includes checking), or a policy classification", text),
		query("releaseClass", "Parsed release type", text), query("resolution", "Parsed resolution", text), query("source", "Parsed source", text),
		query("hdr", "One HDR format", text), query("group", "Release group, case-insensitive", text), query("policy", "Policy snapshot ID", text))
	upgradesOp := paths["/api/v1/upgrades"].(map[string]any)["get"].(map[string]any)
	upgradesOp["parameters"] = append(upgradesOp["parameters"].([]any),
		query("level", "Upgrade level", map[string]any{"type": "string", "enum": []string{"upgrade", "possible"}}),
		query("monitorId", "One monitor", text),
		query("q", "Torrent or offered release name, or either group", map[string]any{"type": "string", "maxLength": 200}),
		query("dismissed", "List dismissed offers instead", map[string]any{"type": "boolean"}))
	idxOp := paths["/api/v1/indexers"].(map[string]any)["get"].(map[string]any)
	idxOp["parameters"] = []any{map[string]any{"name": "refresh", "in": "query", "schema": map[string]any{"type": "boolean"}}}
	op := paths["/api/v1/searches/{id}/events"].(map[string]any)["get"].(map[string]any)
	op["parameters"] = append(op["parameters"].([]any), map[string]any{"name": "Last-Event-ID", "in": "header", "schema": map[string]any{"type": "integer", "minimum": 0}})
	op["responses"].(map[string]any)["200"] = map[string]any{"description": "Each data field contains a Job. Events: search.started, indexer.started, indexer.completed, indexer.failed, analysis.started, analysis.completed, search.completed, search.snapshot.", "content": map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}}}}
	settings := schemas["Settings"].(map[string]any)
	settings["properties"].(map[string]any)["apiKey"].(map[string]any)["writeOnly"] = true
	settings["required"] = []string{}
	for _, item := range []map[string]any{schemas["MetadataSettings"].(map[string]any), settings["properties"].(map[string]any)["metadata"].(map[string]any)} {
		item["required"] = []string{}
		props := item["properties"].(map[string]any)
		props["token"].(map[string]any)["writeOnly"] = true
		props["hasToken"].(map[string]any)["readOnly"] = true
		props["environmentManaged"].(map[string]any)["readOnly"] = true
	}
	for _, name := range []string{"hasApiKey", "environmentManaged"} {
		settings["properties"].(map[string]any)[name].(map[string]any)["readOnly"] = true
	}
	request := schemas["SearchRequest"].(map[string]any)
	request["required"] = []string{"media"}
	media := request["properties"].(map[string]any)["media"].(map[string]any)
	media["required"] = []string{"type"}
	media["anyOf"] = []any{map[string]any{"required": []string{"imdbId"}}, map[string]any{"required": []string{"tmdbId"}}}
	media["description"] = "At least one ID is required. Multiple IDs must be verified by TMDb. Other identity fields are resolved by the server."
	mediaProps := media["properties"].(map[string]any)
	mediaProps["type"].(map[string]any)["enum"] = []string{"movie", "tv"}
	mediaProps["imdbId"].(map[string]any)["pattern"] = `^tt[0-9]{7,10}$`
	mediaProps["tmdbId"].(map[string]any)["minimum"] = 1
	mediaProps["season"].(map[string]any)["minimum"] = 0
	mediaProps["season"].(map[string]any)["maximum"] = 999
	mediaProps["season"].(map[string]any)["description"] = "TV searches only"
	mediaProps["episode"].(map[string]any)["minimum"] = 1
	mediaProps["episode"].(map[string]any)["maximum"] = 9999
	mediaProps["episode"].(map[string]any)["description"] = "TV searches with a season only"
	matchingProps := schemas["MatchingRules"].(map[string]any)["properties"].(map[string]any)
	matchingProps["sizeTolerancePercent"].(map[string]any)["minimum"] = 0
	matchingProps["sizeTolerancePercent"].(map[string]any)["maximum"] = 1
	return map[string]any{"openapi": "3.0.3", "info": map[string]any{"title": "RavenSeed API", "version": "0.6.0", "description": "Reported metadata and byte-size matching only. Authenticated deployments require a browser session or API bearer token. Cookie-authenticated mutations require the same-origin Origin header. The empty security alternative applies only when authentication is disabled."}, "paths": paths, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"}, "browserSession": map[string]any{"type": "apiKey", "in": "cookie", "name": "__Host-ravenseed_session"}, "localBrowserSession": map[string]any{"type": "apiKey", "in": "cookie", "name": "ravenseed_session"}}}}
}
