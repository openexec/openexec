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

  useEffect(() => {
    const fetchData = async () => {
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
      } catch (e) {
        console.error("Failed to load knowledge", e);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  if (loading) return <div style={styles.container}>Loading Deterministic Knowledge Base...</div>;

  return (
    <div style={styles.container}>
      <h1 style={styles.title}>Deterministic Control Plane</h1>
      <p>This dashboard exposes the structured "Pointer Records" used by the OpenExec Orchestrator for zero-hallucination planning and execution.</p>

      <div style={styles.section}>
        <h2>Surgical Pointers (Symbols)</h2>
        <p>Extracted via <code style={styles.code}>openexec knowledge index</code>. The AI reads exactly these lines when modifying functions.</p>
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
        <h2>Environment Topologies</h2>
        <p>Hard constraints and IPs for deterministic deployments.</p>
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
        <h2>Hard Policy Gates</h2>
        <p>Rules that the <code>safe_commit</code> tool enforces before saving code.</p>
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
