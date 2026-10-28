export type SearchMethod = "auto" | "imdb" | "tmdb" | "tvdb" | "title";
export interface IndexerSearchOverride {
  movie: Exclude<SearchMethod, "tvdb">;
  tv: SearchMethod;
}
export interface Indexer {
  searchOverride?: IndexerSearchOverride;
  id: number;
  name: string;
  enabled: boolean;
  backend: string;
  capabilities: string[];
}
export interface Settings {
  searchOverrides?: Record<string, IndexerSearchOverride>;
  metadata?: MetadataSettings;
  quiUrl: string;
  hasApiKey: boolean;
  defaultTargetIndexer: number;
  defaultMinCoverage: number;
  environmentManaged: boolean;
  apiKey?: string;
  matching?: MatchingRules;
}
export interface Media {
  tvdbId?: number;
  title?: string;
  originalTitle?: string;
  year?: number;
  type: "movie" | "tv";
  imdbId?: string;
  tmdbId?: number;
  season?: number;
  episode?: number;
}
export interface SearchRequest {
  media: Media;
  indexerIds: number[];
  targetIndexerId: number;
  filters: { minimumCoverage: number };
  refresh?: boolean;
}
export interface Job {
  media?: MediaSnapshot;
  id: string;
  status: string;
  request: SearchRequest;
  startedAt: string;
  completedAt?: string;
  indexers: {
    indexerId: number;
    name: string;
    status: string;
    resultCount: number;
    error?: string;
    strategy?: { method: string; query: string; reason?: string };
  }[];
  indexerErrors: { indexerId: number; error: string }[];
  resultCount: number;
  familyCount: number;
  candidateCount: number;
  error?: string;
  cacheHit: boolean;
  matchingRules?: MatchingRules;
}
export interface Parsed {
  checksum?: string;
  mediaType: string;
  title: string;
  year: number;
  season: number;
  episode: number;
  structure: string;
  resolution: string;
  releaseType: string;
  source: string;
  videoCodec: string;
  bitDepth: string;
  hdr: string[];
  audioCodec: string;
  audioChannels: string;
  edition: string[];
  cut: string[];
  group: string;
  displayGroup: string;
  repack: boolean;
  proper: boolean;
  correction?: string;
  revision?: number;
  hybrid?: boolean;
  service?: string;
  contentId?: string;
  seasons?: number[];
  episodes?: number[];
  absoluteEpisodes?: number[];
}
export interface Result {
  id: string;
  indexerId: number;
  indexer: string;
  title: string;
  size: number;
  seeders: number;
  publishDate: string;
  infoUrl: string;
  /** The server keeps the download link (it may carry a passkey) and
   * resolves it by result ID when adding the torrent. */
  hasDownloadUrl: boolean;
  downloadFactor?: number | null;
  uploadFactor?: number | null;
  parsed: Parsed;
  confidence: string;
  evidence: string[];
  normalizationEvidence?: string[];
  identityEvidence?: string[];
  identityVerified: boolean;
  raw: unknown;
  parserOutput: unknown;
}

export interface IndexerPreset {
  id: string;
  name: string;
  indexerIds: number[];
  targetIndexerId: number;
}

export interface QuiInstance {
  id: number;
  name: string;
  connected: boolean;
  isActive: boolean;
}

export interface TorrentSettings {
  defaultInstanceId: number;
  categories: { instanceId: number; enabled: string[] }[];
}

export interface UploadFlagPreset {
  id: string;
  name: string;
  args: string;
}

export interface ArrInstance {
  id: string;
  name: string;
  kind: "sonarr" | "radarr";
  url: string;
  apiKey?: string;
  hasApiKey: boolean;
  enabled: boolean;
}
export interface ArrSettings {
  instances: ArrInstance[];
}
export type IdentitySource =
  "arr_history" | "arr_parse" | "arr_lookup" | "none";
export interface TorrentIdentity {
  hash: string;
  source: IdentitySource;
  instance?: string;
  /** Sonarr/Radarr instance ID; names may repeat. */
  instanceId?: string;
  media: Media;
  /** Cross-seeded copy whose download history matched. */
  viaHash?: string;
  /** Imported file matched by name and exact size. */
  importedFile?: string;
  resolvedAt: string;
  expiresAt: string;
}
export interface UploadSettings {
  flagPresets?: UploadFlagPreset[];
  defaultFlagPresetId?: string;
  enabled: boolean;
  url: string;
  token?: string;
  hasToken: boolean;
  clearToken?: boolean;
  trackers: { indexerId: number; code: string }[];
  paths: { instanceId: number; from: string; to: string }[];
}

/** The target tracker's result as Upload-Assistant reported it. */
export interface TrackerResult {
  status: "success" | "failed" | "skipped" | "dupe" | "banned";
  message?: string;
  link?: string;
  torrentId?: string;
}

export interface UploadJob {
  attempt?: number;
  media?: Media;
  previousAttempts?: {
    attempt: number;
    args?: string;
    media: Media;
    status: string;
    message: string;
    trackerResult?: TrackerResult;
    updatedAt: string;
  }[];
  trackerResult?: TrackerResult;
  flagPresetName?: string;
  args?: string;
  id: string;
  title: string;
  tracker: string;
  status: string;
  message: string;
  progress: number;
  instanceId: number;
  hash?: string;
  contentPath?: string;
  uploadPath?: string;
  createdAt: string;
  updatedAt: string;
}
export interface TrackerHost {
  host: string;
  count: number;
  instanceCounts: Record<number, number>;
}
export interface Monitor {
  id: string;
  sourceIndexerId: number;
  sourceIndexerName: string;
  sourceTrackerHost: string;
  targetIndexerId: number;
  targetIndexerName: string;
  intervalHours: number;
  enabled: boolean;
  createdAt: string;
  lastScanAt?: string;
  lastError?: string;
  /** Missing on monitors saved before instance selection: every instance. */
  instanceIds?: number[];
}
export interface MonitorCandidate {
  targetQuery?: string;
  targetResultCount?: number;
  scopedResultCount?: number;
  policy?: PolicyEvaluation;
  parsed?: Parsed;
  id: string;
  monitorId: string;
  instanceId: number;
  hash: string;
  name: string;
  size: number;
  status: string;
  message: string;
  media: Media;
  identity?: TorrentIdentity;
  identityError?: string;
  /** "upgrade" when a policy rule ranks the target's release higher. */
  upgrade?: "upgrade" | "possible";
  upgradeOffer?: PolicyTargetResult;
  nextCheckAt: string;
  lastCheckedAt?: string;
  matchCount: number;
  uploadJobId?: string;
}
export interface Family {
  policy?: PolicyEvaluation;
  relatedFamilies?: {
    familyId: string;
    confidence: string;
    evidence: string[];
  }[];
  id: string;
  canonical: Parsed;
  results: Result[];
  variants: {
    policy?: PolicyEvaluation;
    size: number;
    coverage: number;
    indexerIds: number[];
    resultIds: string[];
    confidence: string;
    targetStatus?: TargetStatus;
  }[];
  coverage: number;
  totalCoverage: number;
  confidence: string;
  evidence: string[];
  seeders: number;
  targetStatus: TargetStatus;
}

export type PolicyClassification =
  | "equivalent"
  | "missing_slot"
  | "coexistence"
  | "trump"
  | "possible_trump"
  | "not_eligible"
  | "unevaluated";

export interface SlotAxes {
  releaseClass: boolean;
  resolution: boolean;
  source: boolean;
  provider: boolean;
  codec: boolean;
  hdr: boolean;
  cut: boolean;
  tvStructure: boolean;
  group: boolean;
  hybrid: boolean;
}

export interface IndexerPolicy {
  indexerId: number;
  enabled: boolean;
  detectMissing: boolean;
  detectCoexistence: boolean;
  detectTrump: boolean;
  detectReview: boolean;
  slots: Record<string, SlotAxes>;
  encodeSizePercent: number;
  /** "1080p:AVC" pairs the size boundary applies to; empty means every encode. */
  encodeSizeScope?: string[];
  /** Maximum size-separated encode slots; 0 is unlimited. */
  encodeSizeSlots?: number;
  encodeBitrateTiers?: { resolution: string; maxVideoMbps: number }[];
  episodesBlockedByPack?: boolean;
  reviewDualAudio?: boolean;
  reviewLosslessWeb?: boolean;
  /** Groups this tracker refuses: not offered, and their releases can be trumped. */
  bannedGroups?: string[];
  /** This tracker's own groups; internal status exists only on its home tracker. */
  internalGroups?: string[];
  internalSlots?: "" | "separate" | "review";
  corrections: string[];
  webDlOverWebRip: boolean;
  sourcePriority: string[];
  hdrRelations: { from: string; to: string }[];
  groupOverNoGrp: boolean;
  reputableGroups: string[];
  seasonPackOverEpisodes: boolean;
  deadTorrentMaxSeeders?: number;
  reviewVisualQuality: boolean;
  reviewHDRComparability: boolean;
  reviewSuperiorMaster: boolean;
  presetId?: string;
  snapshotId?: string;
  updatedAt?: string;
}

export interface PolicyPreset {
  id: string;
  name: string;
  description: string;
  policy: IndexerPolicy;
}

export type PolicyTargetResult = NonNullable<
  PolicyEvaluation["targetResults"]
>[number];

export interface PolicyEvaluation {
  classification: PolicyClassification;
  slotKey?: string;
  slotComponents?: Record<string, string>;
  automaticEvidence: { rule: string; detail: string; capability: string }[];
  missingEvidence: { rule: string; detail: string; capability: string }[];
  reviewReasons: string[];
  competingResultIds: string[];
  targetResults?: {
    id: string;
    title: string;
    indexer: string;
    size: number;
    infoUrl?: string;
    relation: string;
    group?: string;
    upgrade?: { type: string; from: string; to: string }[];
    upgradeReview?: string;
  }[];
  improvements: { type: string; from: string; to: string }[];
  regressions: { type: string; from: string; to: string }[];
  reason: string;
  policySnapshotId?: string;
  evaluatedAt: string;
}

export interface TargetStatus {
  indexerId: number;
  evaluated: boolean;
  candidate: boolean;
  exactMatchFound: boolean;
  relatedMatchFound: boolean;
  message: string;
}
export interface MetadataSettings {
  token?: string;
  hasToken: boolean;
  environmentManaged: boolean;
  language: string;
  clearToken?: boolean;
}
export interface SeasonMetadata {
  number: number;
  name: string;
  airDate: string;
  episodeCount: number;
  overview: string;
  posterPath: string;
}
export interface MediaSnapshot extends Media {
  overview?: string;
  posterPath?: string;
  backdropPath?: string;
  seasons?: SeasonMetadata[];
  seasonDetails?: SeasonMetadata;
  fetchedAt: string;
  expiresAt: string;
  cached: boolean;
  warning?: string;
}
export interface Page<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

/** GET /uploads: `total` counts the status filter; `statusCounts` counts
 * every job in the queue by status. */
export interface UploadJobsPage extends Page<UploadJob> {
  statusCounts: Record<string, number>;
}

export interface EpisodeMapping {
  imdbId?: string;
  tmdbId?: number;
  absolute: number;
  season: number;
  episode: number;
}

export interface MatchingRules {
  matchAlternateTitles: boolean;
  sizeTolerancePercent: number;
  groupAliases: Record<string, string>;
  ignoredTags: string[];
  episodeMappings: EpisodeMapping[];
}
