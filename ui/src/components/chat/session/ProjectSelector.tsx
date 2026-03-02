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
  /** Whether projects are loading */
  loading?: boolean
}

const ProjectSelector: React.FC<ProjectSelectorProps> = ({
  projects,
  selectedProjectPath,
  onProjectSelect,
  loading = false,
}) => {
  return (
    <div className="project-selector" style={styles.container}>
      <label htmlFor="project-select" style={styles.label}>
        Project Workspace
      </label>
      <select
        id="project-select"
        value={selectedProjectPath || ''}
        onChange={(e) => onProjectSelect(e.target.value)}
        disabled={loading}
        style={styles.select}
      >
        <option value="">All Projects</option>
        {projects.map((project) => (
          <option key={project.path} value={project.path}>
            {project.name}
          </option>
        ))}
      </select>
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
    gap: '6px',
  },
  label: {
    fontSize: '12px',
    fontWeight: 500,
    color: '#8b949e',
  },
  select: {
    width: '100%',
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
