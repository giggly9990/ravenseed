import { iconGallery } from "../../components/icons";

export function DevIconsPage() {
  return (
    <>
      <header className="page-header">
        <h1>Glyphs</h1>
        <span className="count-badge">{iconGallery.length}</span>
        <span className="hint">
          Registry keys for code and docs · 16 / 20 / 24px · currentColor
        </span>
      </header>
      <ul className="dev-icon-grid" aria-label="Icons">
        {iconGallery.map(([name, Icon]) => (
          <li className="panel dev-icon-card" key={name}>
            <div className="dev-icon-preview" aria-hidden="true">
              <Icon size={16} />
              <Icon size={20} />
              <Icon size={24} />
            </div>
            <code className="mono small">{name}</code>
          </li>
        ))}
      </ul>
    </>
  );
}
