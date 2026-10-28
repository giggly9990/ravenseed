import type { ReactNode } from "react";
import { RefreshIcon } from "../../components/icons";
import type { Media, MediaSnapshot } from "../../types";

const tmdbImage = (path: string | undefined, size: string) =>
  path && /^\/[a-zA-Z0-9_-]+\.(jpg|png|webp)$/.test(path)
    ? `https://image.tmdb.org/t/p/${size}${path}`
    : undefined;

/**
 * One strip for the title being searched: poster, title, one-line overview
 * and IDs on the left, whatever the page needs (search status) on the right.
 */
export function TitleStrip({
  media,
  request,
  refresh,
  refreshing = false,
  label = "Title and search summary",
  children,
}: {
  media?: MediaSnapshot;
  /** The searched identity, used when metadata is unavailable. */
  request?: Media;
  refresh?: () => void;
  refreshing?: boolean;
  label?: string;
  children?: ReactNode;
}) {
  const known = media?.title ? media : undefined;
  const ids = { ...request, ...known };
  const season = known?.seasonDetails;
  const poster = tmdbImage(season?.posterPath || known?.posterPath, "w154");
  const type = (known?.type ?? request?.type) === "tv" ? "TV" : "Movie";
  const seasonNumber = known?.season ?? request?.season;
  const overview = season?.overview || known?.overview;
  return (
    <section className="panel disc-strip" aria-label={label}>
      <div className="disc-poster">
        {poster && (
          <img
            src={poster}
            alt={`${known?.title} poster`}
            onError={(e) => {
              e.currentTarget.hidden = true;
            }}
          />
        )}
      </div>
      <div className="disc-title">
        <div className="disc-title-line">
          <h1>
            {known?.title ||
              (ids.imdbId ?? (ids.tmdbId ? `TMDb ${ids.tmdbId}` : "Untitled"))}
          </h1>
          <span className="muted">
            {[
              known?.year,
              type,
              seasonNumber !== undefined &&
                (season?.name || `Season ${seasonNumber}`) +
                  (season ? ` · ${season.episodeCount} episodes` : ""),
              request?.episode !== undefined && `Episode ${request.episode}`,
            ]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </div>
        {known?.originalTitle && known.originalTitle !== known.title && (
          <p className="disc-original truncate">{known.originalTitle}</p>
        )}
        {overview ? (
          <p className="disc-overview truncate" title={overview}>
            {overview}
          </p>
        ) : (
          !known && <p className="disc-overview">Media metadata unavailable.</p>
        )}
        <div className="disc-ids">
          {ids.imdbId && <span>IMDb {ids.imdbId}</span>}
          {ids.tmdbId && <span>TMDb {ids.tmdbId}</span>}
          {ids.tvdbId && <span>TVDb {ids.tvdbId}</span>}
          {known && (
            <a
              href={`https://www.themoviedb.org/${known.type}/${known.tmdbId}`}
              target="_blank"
              rel="noreferrer"
              title="This product uses the TMDB API but is not endorsed or certified by TMDB."
            >
              Metadata from TMDb{known.cached ? " · cached" : ""}
            </a>
          )}
          {refresh && (
            <button
              type="button"
              className="link-button"
              onClick={refresh}
              disabled={refreshing}
            >
              <RefreshIcon size={11} className={refreshing ? "spin" : ""} />
              Refresh metadata
            </button>
          )}
        </div>
        {media?.warning && (
          <p className="hint" role="status">
            {media.warning}
          </p>
        )}
      </div>
      {children && <div className="disc-strip-side">{children}</div>}
    </section>
  );
}
