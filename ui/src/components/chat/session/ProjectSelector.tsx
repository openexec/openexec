/**
 * ProjectSelector Component
 *
 * Dropdown selector for choosing the active project workspace.
 *
 * @module components/chat/session/ProjectSelector
 */

import React from 'react'
import type { ProjectInfo } from '../../../types/chat'

export interface ProjectSelectorProps {
  /** List of available projects */
  projects: ProjectInfo[]
  /** Currently selected project path */
  selectedProjectPath?: string
  /** Callback when a project is selected */
  onProjectSelect: (projectPath: string) => void
  /** Callback to init a new project */
  onProjectInit: () => void
  /** Callback to start wizard */
  onProjectWizard: () => void
  /** Whether projects are loading */
  loading?: boolean
}

const ProjectSelector: React.FC<ProjectSelectorProps> = ({
  projects,
  selectedProjectPath,
  onProjectSelect,
  onProjectInit,
  onProjectWizard,
  loading = false,
}) => {
  return (
    <div className="project-selector" style={styles.container}>
      <label htmlFor="project-select" style={styles.label}>
        Project Workspace
      </label>
      <div style={styles.selectorRow}>
        <select
          id="project-select"
          value={selectedProjectPath || ''}
          onChange={(e) => onProjectSelect(e.target.value)}
          disabled={loading}
          style={styles.select}
        >
          <option value="">Select Project...</option>
          {projects.map((project) => (
            <option key={project.path} value={project.path}>
              {project.name}
            </option>
          ))}
        </select>
      </div>
      
      <div style={styles.actionsRow}>
        <button 
          onClick={onProjectInit} 
          style={styles.actionButton}
          title="Initialize new project in a directory"
        >
          Init
        </button>
        <button 
          onClick={onProjectWizard} 
          style={{...styles.actionButton, ...styles.wizardButton}}
          disabled={!selectedProjectPath}
          title="Start guided setup wizard for selected project"
        >
          Wizard
        </button>
      </div>

      {loading && <span style={styles.loading}>Loading...</span>}
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    padding: '12px 16px',
    borderBottom: '1px solid #30363d',
    display: 'flex',
    flexDirection: 'column',
    gap: '10px',
  },
  selectorRow: {
    display: 'flex',
    gap: '8px',
  },
  actionsRow: {
    display: 'flex',
    gap: '8px',
  },
  actionButton: {
    flex: 1,
    padding: '4px 8px',
    fontSize: '11px',
    fontWeight: 600,
    color: '#c9d1d9',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '4px',
    cursor: 'pointer',
  },
  wizardButton: {
    backgroundColor: '#30363d',
    borderColor: '#8b949e',
  },
  label: {
    fontSize: '11px',
    fontWeight: 600,
    color: '#8b949e',
    textTransform: 'uppercase',
  },
  select: {
    flex: 1,
    padding: '6px 10px',
    fontSize: '13px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    outline: 'none',
    cursor: 'pointer',
  },
  loading: {
    fontSize: '11px',
    color: '#8b949e',
    marginTop: '2px',
  },
}

export default ProjectSelector
