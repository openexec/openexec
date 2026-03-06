import React, { useEffect, useState } from 'react';

interface SymbolRecord {
  name: string;
  kind: string;
  file_path: string;
  start_line: number;
  end_line: number;
  purpose: string;
}

interface EnvironmentRecord {
  env: string;
  runtime_type: string;
  topology: string;
}

interface PolicyRecord {
  key: string;
  value: string;
  description: string;
}

const styles = {
  container: {
    padding: '2rem',
    maxWidth: '1200px',
    margin: '0 auto',
    color: '#c9d1d9',
  },
  title: {
    color: '#58a6ff',
    borderBottom: '1px solid #30363d',
    paddingBottom: '0.5rem',
  },
  section: {
    marginTop: '2rem',
    backgroundColor: '#161b22',
    padding: '1.5rem',
    borderRadius: '6px',
    border: '1px solid #30363d',
  },
  list: {
    listStyle: 'none',
    padding: 0,
  },
  item: {
    padding: '1rem',
    borderBottom: '1px solid #21262d',
  },
  code: {
    backgroundColor: '#0d1117',
    padding: '0.2rem 0.4rem',
    borderRadius: '3px',
    fontFamily: 'monospace',
    color: '#8b949e',
  }
};

export const KnowledgeHub: React.FC = () => {
  const [symbols, setSymbols] = useState<SymbolRecord[]>([]);
  const [envs, setEnvs] = useState<EnvironmentRecord[]>([]);
  const [policies, setPolicies] = useState<PolicyRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchData = async () => {
    setError(null)
    setLoading(true)
    try {
      const [symRes, envRes, polRes] = await Promise.all([
        fetch('/api/v1/knowledge/symbols'),
        fetch('/api/v1/knowledge/envs'),
        fetch('/api/v1/knowledge/policies')
      ]);

      if (symRes.ok) {
        const data = await symRes.json();
        setSymbols(data.symbols || []);
      }
      if (envRes.ok) {
        const data = await envRes.json();
        setEnvs(data.environments || []);
      }
      if (polRes.ok) {
        const data = await polRes.json();
        setPolicies(data.policies || []);
      }

      if (!symRes.ok || !envRes.ok || !polRes.ok) {
        setError('Some knowledge endpoints failed to load')
      }
    } catch (e: any) {
      console.error('Failed to load knowledge', e);
      setError(e?.message || 'Failed to load knowledge')
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void fetchData()
  }, []);

  if (loading) return <div style={styles.container}>Loading Project Knowledge Map...</div>;

  return (
    <div style={styles.container}>
      <h1 style={styles.title}>Project Knowledge Map</h1>
      <p>This dashboard shows the structured "Code Map" used by OpenExec to ensure the AI knows exactly what to change with zero-hallucination.</p>

      {error && (
        <div role="alert" style={{ marginTop: '1rem', padding: '0.75rem 1rem', border: '1px solid #8b949e', borderRadius: 6, background: '#0d1117' }}>
          <span style={{ color: '#f85149' }}>Error:</span> {error}
          <button onClick={() => void fetchData()} style={{ marginLeft: 12, border: '1px solid #30363d', background: '#21262d', color: '#c9d1d9', padding: '4px 8px', borderRadius: 4, cursor: 'pointer' }}>
            Retry
          </button>
        </div>
      )}

      <div style={styles.section}>
        <h2>Surgical Code Map (Symbols)</h2>
        <p>The AI reads exactly these lines when modifying functions, ensuring precision and reducing token usage.</p>
        <ul style={styles.list}>
          {symbols.length === 0 ? <li style={styles.item}>No symbols indexed.</li> : 
            symbols.map(s => (
              <li key={s.name} style={styles.item}>
                <strong>{s.name}</strong> <span style={styles.code}>{s.kind}</span>
                <br/>
                <small style={{color: '#8b949e'}}>📍 {s.file_path} (L{s.start_line}-L{s.end_line})</small>
                <p style={{margin: '0.5rem 0 0 0'}}>{s.purpose}</p>
              </li>
            ))
          }
        </ul>
      </div>

      <div style={styles.section}>
        <h2>Environment Maps</h2>
        <p>Hard constraints and network maps for reliable deployments.</p>
        <ul style={styles.list}>
          {envs.length === 0 ? <li style={styles.item}>No environments recorded.</li> : 
            envs.map(e => (
              <li key={e.env} style={styles.item}>
                <strong>{e.env.toUpperCase()}</strong> <span style={styles.code}>{e.runtime_type}</span>
                <pre style={{...styles.code, padding: '1rem', marginTop: '0.5rem'}}>{e.topology}</pre>
              </li>
            ))
          }
        </ul>
      </div>

      <div style={styles.section}>
        <h2>Safety Guardrails (Policies)</h2>
        <p>Rules that OpenExec enforces automatically before saving any code.</p>
        <ul style={styles.list}>
          {policies.length === 0 ? <li style={styles.item}>No policies recorded.</li> : 
            policies.map(p => (
              <li key={p.key} style={styles.item}>
                <strong>{p.key}</strong>
                <pre style={{...styles.code, padding: '1rem', marginTop: '0.5rem'}}>{p.value}</pre>
                <p>{p.description}</p>
              </li>
            ))
          }
        </ul>
      </div>
    </div>
  );
};

export default KnowledgeHub;
