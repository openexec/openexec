package governance

// Schema defines the SQLite tables for release governance. All tables are new and
// additive; nothing here touches the existing releases/stories/tasks/goals tables.
// JSON-encoded slices are stored as TEXT DEFAULT '[]'. Timestamps are DATETIME
// stored as RFC3339 strings.
const Schema = `
-- governance_releases stores durable, multi-instance release records.
CREATE TABLE IF NOT EXISTS governance_releases (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL DEFAULT '',
	description TEXT DEFAULT '',
	owner TEXT DEFAULT '',
	status TEXT NOT NULL DEFAULT 'draft',
	goal TEXT DEFAULT '',
	must_have TEXT DEFAULT '[]',
	out_of_scope TEXT DEFAULT '[]',
	risk TEXT DEFAULT 'low',
	approved_for_ai INTEGER DEFAULT 0,
	approved_version INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_governance_releases_status ON governance_releases(status);
CREATE INDEX IF NOT EXISTS idx_governance_releases_owner ON governance_releases(owner);
CREATE INDEX IF NOT EXISTS idx_governance_releases_updated_at ON governance_releases(updated_at);

-- change_records bridge source-system items to release items, plans, PRs, and evidence.
CREATE TABLE IF NOT EXISTS change_records (
	id TEXT PRIMARY KEY,
	release_id TEXT DEFAULT '',
	project_id TEXT DEFAULT '',
	source_type TEXT DEFAULT 'manual',
	source_id TEXT DEFAULT '',
	source_url TEXT DEFAULT '',
	title TEXT DEFAULT '',
	raw_text TEXT DEFAULT '',
	summary TEXT DEFAULT '',
	kind TEXT DEFAULT '',
	risk TEXT DEFAULT 'low',
	status TEXT NOT NULL DEFAULT 'candidate',
	proposal_version INTEGER DEFAULT 0,
	approved_version INTEGER DEFAULT 0,
	plan TEXT DEFAULT '',
	acceptance_criteria TEXT DEFAULT '',
	verification_plan TEXT DEFAULT '',
	branch TEXT DEFAULT '',
	pr_url TEXT DEFAULT '',
	claimed_by TEXT DEFAULT '',
	claim_expires_at DATETIME DEFAULT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Partial UNIQUE: only enforced for source-backed records (non-empty source_id).
-- Without the WHERE clause, two manual records (both source_id='') would collide.
CREATE UNIQUE INDEX IF NOT EXISTS idx_change_records_source ON change_records(source_type, source_id, project_id) WHERE source_id != '';
CREATE INDEX IF NOT EXISTS idx_change_records_release_id ON change_records(release_id);
CREATE INDEX IF NOT EXISTS idx_change_records_status ON change_records(status);
CREATE INDEX IF NOT EXISTS idx_change_records_risk ON change_records(risk);
CREATE INDEX IF NOT EXISTS idx_change_records_updated_at ON change_records(updated_at);

-- release_items link change records to releases.
CREATE TABLE IF NOT EXISTS release_items (
	release_id TEXT NOT NULL,
	change_id TEXT NOT NULL,
	item_type TEXT DEFAULT '',
	priority INTEGER DEFAULT 0,
	required INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_release_items_release_change ON release_items(release_id, change_id);

-- decision_events record every governance decision against a change.
CREATE TABLE IF NOT EXISTS decision_events (
	id TEXT PRIMARY KEY,
	release_id TEXT DEFAULT '',
	change_id TEXT DEFAULT '',
	proposal_version INTEGER DEFAULT 0,
	actor TEXT DEFAULT '',
	actor_type TEXT DEFAULT '',
	decision TEXT DEFAULT '',
	comment TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_decision_events_release_id ON decision_events(release_id);
CREATE INDEX IF NOT EXISTS idx_decision_events_change_id ON decision_events(change_id);
CREATE INDEX IF NOT EXISTS idx_decision_events_created_at ON decision_events(created_at);

-- review_authorities define what each actor may do.
CREATE TABLE IF NOT EXISTS review_authorities (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL DEFAULT '',
	type TEXT DEFAULT '',
	permissions TEXT DEFAULT '[]',
	risk_limit TEXT DEFAULT 'low'
);

-- evidence stores structured proof attached to a change.
CREATE TABLE IF NOT EXISTS evidence (
	id TEXT PRIMARY KEY,
	change_id TEXT DEFAULT '',
	kind TEXT DEFAULT '',
	source TEXT DEFAULT '',
	summary TEXT DEFAULT '',
	url TEXT DEFAULT '',
	raw TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_evidence_change_id ON evidence(change_id);

-- communication_artifacts store audience-targeted output generated from records.
CREATE TABLE IF NOT EXISTS communication_artifacts (
	id TEXT PRIMARY KEY,
	release_id TEXT DEFAULT '',
	change_id TEXT DEFAULT '',
	audience TEXT DEFAULT '',
	body TEXT DEFAULT '',
	posted_to TEXT DEFAULT '',
	posted_at DATETIME DEFAULT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_communication_artifacts_release_id ON communication_artifacts(release_id);
CREATE INDEX IF NOT EXISTS idx_communication_artifacts_change_id ON communication_artifacts(change_id);
`
