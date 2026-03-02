/**
 * ProjectInitModal Component
 *
 * Modal for initializing a new project in a directory.
 *
 * @module components/chat/session/ProjectInitModal
 */

import React, { useState, useCallback } from 'react'
import DirectoryPicker from './DirectoryPicker'

export interface ProjectInitModalProps {
  /** Callback when initialization is submitted */
  onSubmit: (name: string, path: string) => void
  /** Callback to close modal */
  onCancel: () => void
  /** Base API URL */
  apiUrl: string
  /** Loading state */
  loading?: boolean
}

const ProjectInitModal: React.FC<ProjectInitModalProps> = ({ onSubmit, onCancel, apiUrl, loading }) => {
  const [name, setName] = useState('')
  const [path, setPath] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (path.trim()) {
      onSubmit(name.trim(), path.trim())
    }
  }

  return (
    <div style={styles.overlay} onClick={onCancel}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <h3 style={styles.title}>Initialize Project</h3>
        <p style={styles.description}>Create a new OpenExec project workspace in a local directory.</p>

        <form onSubmit={handleSubmit}>
          <div style={styles.field}>
            <label style={styles.label}>Project Name (optional)</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. my-new-app"
              style={styles.input}
              disabled={loading}
            />
          </div>

          <div style={styles.field}>
            <label style={styles.label}>Directory Path (required)</label>
            <input
              type="text"
              value={path}
              onChange={e => setPath(e.target.value)}
              placeholder="Select a directory below..."
              style={styles.input}
              required
              disabled={loading}
            />
            
            <DirectoryPicker 
              value={path} 
              onChange={setPath} 
              apiUrl={apiUrl} 
            />
            
            <span style={styles.hint}>Navigate and click folder to select. Path is absolute or relative to projects root.</span>
          </div>

          <div style={styles.actions}>
            <button type="button" onClick={onCancel} style={styles.cancelButton} disabled={loading}>
              Cancel
            </button>
            <button type="submit" style={styles.submitButton} disabled={loading || !path.trim()}>
              {loading ? 'Initializing...' : 'Initialize'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    top: 0, left: 0, right: 0, bottom: 0,
    backgroundColor: 'rgba(0,0,0,0.5)',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    zIndex: 1500,
  },
  modal: {
    backgroundColor: '#161b22',
    borderRadius: '8px', border: '1px solid #30363d',
    padding: '24px', width: '450px', maxWidth: '90vw',
  },
  title: { margin: '0 0 8px 0', color: '#c9d1d9' },
  description: { fontSize: '13px', color: '#8b949e', marginBottom: '20px' },
  field: { marginBottom: '16px' },
  label: { display: 'block', fontSize: '12px', fontWeight: 500, color: '#8b949e', marginBottom: '6px' },
  input: {
    width: '100%', padding: '8px 12px', fontSize: '14px',
    color: '#c9d1d9', backgroundColor: '#0d1117',
    border: '1px solid #30363d', borderRadius: '6px', outline: 'none',
    boxSizing: 'border-box',
  },
  hint: { fontSize: '11px', color: '#8b949e', marginTop: '4px', display: 'block' },
  actions: { display: 'flex', justifyContent: 'flex-end', gap: '12px', marginTop: '24px' },
  cancelButton: {
    padding: '8px 16px', fontSize: '14px', fontWeight: 500,
    color: '#c9d1d9', backgroundColor: '#21262d',
    border: '1px solid #30363d', borderRadius: '6px', cursor: 'pointer',
  },
  submitButton: {
    padding: '8px 16px', fontSize: '14px', fontWeight: 500,
    color: '#ffffff', backgroundColor: '#238636',
    border: 'none', borderRadius: '6px', cursor: 'pointer',
  },
}

export default ProjectInitModal
