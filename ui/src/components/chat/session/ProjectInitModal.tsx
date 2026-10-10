/**
 * ProjectInitModal Component
 *
 * Modal for creating a new project: repository-based (a git-backed directory
 * running the code pipeline) or chat-based (a conversational workspace with no
 * repository, which needs only a name).
 *
 * @module components/chat/session/ProjectInitModal
 */

import React, { useState } from 'react'
import DirectoryPicker from './DirectoryPicker'
import type { ProjectKind } from '../../../types/chat'

export interface ProjectInitModalProps {
  /** Callback when initialization is submitted */
  onSubmit: (name: string, path: string, kind: ProjectKind) => void
  /** Callback to close modal */
  onCancel: () => void
  /** Base API URL */
  apiUrl: string
  /** Loading state */
  loading?: boolean
}

const sanitizeProjectName = (name: string) => {
  return name.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').replace(/^-|-$/g, '')
}

const ProjectInitModal: React.FC<ProjectInitModalProps> = ({ onSubmit, onCancel, apiUrl, loading }) => {
  const [kind, setKind] = useState<ProjectKind>('repository')
  const [name, setName] = useState('')
  const [path, setPath] = useState('')

  const isChat = kind === 'chat'
  const sanitizedName = name.trim() ? sanitizeProjectName(name.trim()) : ''
  const sanitizedPath = path.trim()
  // A repository project lives in a directory; a chat project needs only a name
  const canSubmit = isChat ? Boolean(sanitizedName || sanitizedPath) : Boolean(sanitizedPath)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (canSubmit) {
      onSubmit(sanitizedName, sanitizedPath, kind)
    }
  }

  return (
    <div style={styles.overlay} onClick={onCancel}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <h3 style={styles.title}>New Project</h3>
        <p style={styles.description}>Create a new OpenExec project workspace.</p>

        <form onSubmit={handleSubmit}>
          <fieldset style={styles.kindGroup} disabled={loading}>
            <legend style={styles.label}>Project Type</legend>
            <label style={styles.kindOption}>
              <input
                type="radio"
                name="project-kind"
                value="repository"
                checked={kind === 'repository'}
                onChange={() => setKind('repository')}
              />
              <span>
                <strong>Repository</strong>
                <span style={styles.kindHint}> — code in a git repository; runs the build pipeline</span>
              </span>
            </label>
            <label style={styles.kindOption}>
              <input
                type="radio"
                name="project-kind"
                value="chat"
                checked={isChat}
                onChange={() => setKind('chat')}
              />
              <span>
                <strong>Chat only</strong>
                <span style={styles.kindHint}> — conversations without a repository</span>
              </span>
            </label>
          </fieldset>

          <div style={styles.field}>
            <label style={styles.label} htmlFor="project-name">
              {isChat ? 'Project Name (required unless a directory is given)' : 'Project Name (optional)'}
            </label>
            <input
              id="project-name"
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. my-new-app"
              style={styles.input}
              disabled={loading}
            />
          </div>

          <div style={styles.field}>
            <label style={styles.label} htmlFor="directory-path">
              {isChat ? 'Directory Path (optional)' : 'Directory Path (required)'}
            </label>
            <input
              id="directory-path"
              type="text"
              value={path}
              onChange={e => setPath(e.target.value)}
              placeholder={isChat ? 'Leave empty to create it under the projects root' : 'Select a directory below...'}
              style={styles.input}
              required={!isChat}
              disabled={loading}
            />

            {!isChat && (
              <DirectoryPicker
                value={path}
                onChange={setPath}
                apiUrl={apiUrl}
              />
            )}

            <span style={styles.hint}>
              {isChat
                ? 'A chat project gets no git repository and no code quality gates.'
                : 'Navigate and click folder to select. Path is absolute or relative to projects root. A git repository is created if the directory has none.'}
            </span>
          </div>

          <div style={styles.actions}>
            <button type="button" onClick={onCancel} style={styles.cancelButton} disabled={loading}>
              Cancel
            </button>
            <button type="submit" style={styles.submitButton} disabled={loading || !canSubmit}>
              {loading ? 'Creating...' : 'Create Project'}
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
  kindGroup: { border: 'none', padding: 0, margin: '0 0 16px 0', display: 'flex', flexDirection: 'column', gap: '6px' },
  kindOption: { display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px', color: '#c9d1d9', cursor: 'pointer' },
  kindHint: { color: '#8b949e', fontWeight: 400 },
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
