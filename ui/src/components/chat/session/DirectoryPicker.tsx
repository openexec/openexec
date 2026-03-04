/**
 * DirectoryPicker Component
 *
 * Allows browsing and selecting local directories.
 *
 * @module components/chat/session/DirectoryPicker
 */

import React, { useState, useEffect, useCallback } from 'react'

export interface DirectoryInfo {
  name: string
  path: string
}

export interface DirectoryPickerProps {
  /** Currently selected path */
  value: string
  /** Callback when path changes */
  onChange: (path: string) => void
  /** Base API URL */
  apiUrl: string
}

const DirectoryPicker: React.FC<DirectoryPickerProps> = ({ value, onChange, apiUrl }) => {
  const [dirs, setDirs] = useState<DirectoryInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | undefined>()

  const fetchDirs = useCallback(async (path: string) => {
    setLoading(true)
    setError(undefined)
    try {
      const response = await fetch(`${apiUrl}/directories?path=${encodeURIComponent(path)}`)
      if (!response.ok) throw new Error('Failed to fetch directories')
      const data = await response.json()
      setDirs(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }, [apiUrl])

  useEffect(() => {
    fetchDirs(value || '')
  }, [value, fetchDirs])

  const handleDirClick = (path: string) => {
    onChange(path)
    fetchDirs(path)
  }

  return (
    <div style={styles.container}>
      <div style={styles.currentPath}>
        <strong>Current:</strong> {value || '(default projects root)'}
      </div>
      
      <div style={styles.list}>
        {loading && <div style={styles.status}>Loading...</div>}
        {error && <div style={styles.error}>{error}</div>}
        
        {!loading && !error && (dirs || []).map((dir) => (
          <div 
            key={dir.path} 
            onClick={() => handleDirClick(dir.path)}
            style={styles.item}
          >
            <span style={styles.icon}>{dir.name === '..' ? '⬆️' : '📁'}</span>
            <span style={styles.name}>{dir.name}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    border: '1px solid #30363d',
    borderRadius: '6px',
    backgroundColor: '#0d1117',
    marginTop: '8px',
    overflow: 'hidden',
  },
  currentPath: {
    padding: '8px 12px',
    fontSize: '12px',
    color: '#c9d1d9',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #30363d',
    wordBreak: 'break-all',
  },
  list: {
    maxHeight: '200px',
    overflowY: 'auto',
    padding: '4px 0',
  },
  item: {
    padding: '6px 12px',
    fontSize: '13px',
    color: '#c9d1d9',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  icon: { fontSize: '14px' },
  name: { flex: 1 },
  status: { padding: '12px', color: '#8b949e', textAlign: 'center', fontSize: '13px' },
  error: { padding: '12px', color: '#f85149', textAlign: 'center', fontSize: '13px' },
}

// Add hover effect via CSS injection or just inline style
// We'll stick to basic styles for now

export default DirectoryPicker
