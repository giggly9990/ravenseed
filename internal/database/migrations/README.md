# Migration history

Migrations are ordered and recorded by their full filename. Never rename or edit a shipped SQL migration. Add a new file with the next unused numeric prefix.

Both `002_events.sql` and `002_presets.sql` shipped before this convention was documented. Keep both names so existing databases do not try to apply them again. Upgrade tests cover every existing migration prefix and repeated startup.
