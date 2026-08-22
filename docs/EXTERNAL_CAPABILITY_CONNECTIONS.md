# Governed external capability connections

**Status:** Phase 0 built on this branch. The first provider binding is Lovable
OAuth plus safe identity/catalog discovery. Design mutation and deployment are
not implemented by this slice.

OpenExec owns external MCP credentials, provider discovery, project binding,
effect policy and invocation evidence. Agent Console is only the authenticated
owner-facing control surface. A browser never supplies the authoritative
project binding, OpenExec credential, or OAuth connection identity.

## Phase 0 boundary

The first Lovable connection can:

- authorize against `https://mcp.lovable.dev` through OAuth with PKCE,
  protected-resource discovery, Client ID Metadata Documents (with dynamic
  registration as a compatibility fallback), and resource binding;
- encrypt the OAuth configuration and tokens at rest with AES-256-GCM;
- bind the connection to exactly one Agent Console owner project;
- discover and hash the live MCP catalog;
- call `get_me` as the safe identity/health proof;
- survive an OpenExec restart and repeat that proof; and
- be disabled locally so every later probe is refused before an upstream call.

Lovable grants account-wide authority to its MCP client. OpenExec therefore
does not infer safety from a tool name or MCP annotation. The Phase 0 binding
contains an explicit list of documented read operations. Unknown tools and
all mutation tools, including `create_project`, `send_message`,
`deploy_project`, `query_database`, visibility changes and knowledge writes,
are consequential and unavailable through this slice.

The next slice may introduce bounded prototype creation, but only through a
separate effect contract. Connecting Lovable does not grant that authority.

## Configuration

Both values are required together:

```bash
export OPENEXEC_EXTERNAL_CAPABILITY_TOKEN='<dedicated server-to-server token>'
export OPENEXEC_EXTERNAL_CREDENTIAL_KEY='<base64-encoded random 32-byte key>'
```

The capability token must differ from
`OPENEXEC_REPOSITORY_EVIDENCE_TOKEN` and `OPENEXEC_REPOSITORY_GRAPH_TOKEN`.
The encryption key must remain stable across restarts; losing or changing it
makes stored OAuth credentials unreadable and requires reauthorization.

## Control API

Every route requires `Authorization: Bearer
<OPENEXEC_EXTERNAL_CAPABILITY_TOKEN>`:

- `GET /api/v1/external-connections?project_ref=...`
- `POST /api/v1/external-connections`
- `POST /api/v1/external-connections/{id}/oauth/start`
- `POST /api/v1/external-connections/oauth/callback`
- `POST /api/v1/external-connections/{id}/probe`
- `POST /api/v1/external-connections/{id}/disable`

The OAuth callback accepts only the authorization `code`, random `state`, and
optional issuer. The server-side state selects the connection and project.
Unknown, expired or restart-lost state is refused.

OAuth start also requires a `client_metadata_url`. It must be a non-root HTTPS
URL on the exact callback origin. When the authorization server advertises
Client ID Metadata Document support, OpenExec uses that URL as `client_id`;
the public document, served by Agent Console, binds the callback without a
shared client secret. OpenExec falls back to dynamic registration only when
the authorization server does not advertise the metadata-document mechanism.

## Persisted evidence

The unified SQLite store contains the connection projection, encrypted
credential envelope, project/effect/tool binding, immutable catalog snapshots
and invocation records. A healthy projection includes the catalog digest,
tool count, MCP protocol version, provider server identity and last check time.
The browser never receives credential ciphertext.

## Verification contract

The backend integration test performs the whole local journey against fake
OAuth and Streamable HTTP MCP servers: authorization, catalog discovery,
identity call, encrypted persistence, database close/reopen, restored safe
probe, disable, refused probe and a second restart. Server tests cover bearer
authentication and project binding. The Agent Console test covers the owner
surface and proves browser-supplied project identity is rejected.
