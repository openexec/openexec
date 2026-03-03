/**
 * SessionSidebar Component
 *
 * Left sidebar containing session management functionality.
 * Includes session list, filters, and new session button.
 *
 * @module components/chat/session/SessionSidebar
 */

import React, { useState, useCallback } from 'react'
import type { SessionListItem as SessionListItemType, SessionFilters as SessionFiltersType, CreateSessionParams, ProjectInfo } from '../../../types/chat'
import SessionList from './SessionList'
import SessionFilters from './SessionFilters'
import NewSessionButton from './NewSessionButton'
import ProjectSelector from './ProjectSelector'

export interface SessionSidebarProps {
  /** List of sessions to display */
  sessions: SessionListItemType[]
  /** List of available projects */
  projects?: ProjectInfo[]
  /** Whether projects are loading */
  projectsLoading?: boolean
  /** Currently selected session ID */
  selectedSessionId?: string
  /** Whether sessions are loading */
  loading?: boolean
  /** Callback when a session is selected */
  onSessionSelect?: (sessionId: string) => void
  /** Callback to create a new session */
  onNewSession?: (params: CreateSessionParams) => void
  /** Callback when session filters change */
  onFiltersChange?: (filters: SessionFiltersType) => void
  /** Callback when a project is selected */
  onProjectSelect?: (projectPath: string) => void
  /** Callback when project init is triggered */
  onProjectInit?: () => void
  /** Callback when project wizard is triggered */
  onProjectWizard?: () => void
  /** Callback when fork action is triggered */
  onFork?: (sessionId: string) => void
  /** Callback when archive action is triggered */
  onArchive?: (sessionId: string) => void
  /** Callback when delete action is triggered */
  onDelete?: (sessionId: string) => void
  /** Current filters */
  filters?: SessionFiltersType
  /** Available providers for new session creation */
  providers?: Array<{ id: string; name: string; models: Array<{ id: string; name: string }> }>
  /** Default provider ID for new sessions */
  defaultProvider?: string
  /** Default model ID for new sessions */
  defaultModel?: string
  /** Current project path */
  projectPath?: string
}

const SessionSidebar: React.FC<SessionSidebarProps> = ({
  sessions,
  projects = [],
  projectsLoading = false,
  selectedSessionId,
  loading = false,
  onSessionSelect,
  onNewSession,
  onFiltersChange,
  onProjectSelect,
  onProjectInit,
  onProjectWizard,
  onFork,
  onArchive,
  onDelete,
  filters = {},
  providers = [],
  defaultProvider = 'anthropic',
  defaultModel = 'claude-3-5-sonnet-20241022',
  projectPath = '',
}) => {
  const [showNewSessionModal, setShowNewSessionModal] = useState(false)

  const handleNewSessionClick = useCallback(() => {
    setShowNewSessionModal(true)
  }, [])

  const handleCreateSession = useCallback((params: CreateSessionParams) => {
    onNewSession?.(params)
    setShowNewSessionModal(false)
  }, [onNewSession])

  const handleCancelNewSession = useCallback(() => {
    setShowNewSessionModal(false)
  }, [])

  return (
    <aside className="session-sidebar" style={styles.container}>
      {/* Header with new session button */}
      <div className="session-sidebar__header" style={styles.header}>
        <h2 style={styles.title}>Sessions</h2>
        <NewSessionButton
          onClick={handleNewSessionClick}
          disabled={loading}
        />
      </div>

      {/* Project Selector */}
      <ProjectSelector
        projects={projects}
        selectedProjectPath={projectPath}
        onProjectSelect={onProjectSelect || (() => {})}
        onProjectInit={onProjectInit || (() => {})}
        onProjectWizard={onProjectWizard || (() => {})}
        loading={projectsLoading}
      />

      {/* Filters */}
      <div className="session-sidebar__filters" style={styles.filters}>
        <SessionFilters
          filters={filters}
          onChange={onFiltersChange}
        />
      </div>

      {/* Session list */}
      <div className="session-sidebar__list" style={styles.list}>
        <SessionList
          sessions={sessions}
          selectedId={selectedSessionId}
          onSelect={onSessionSelect}
          onFork={onFork}
          onArchive={onArchive}
          onDelete={onDelete}
          loading={loading}
        />
      </div>

      {/* New Session Modal */}
      {showNewSessionModal && (
        <NewSessionModal
          providers={providers}
          defaultProvider={defaultProvider}
          defaultModel={defaultModel}
          projectPath={projectPath}
          onSubmit={handleCreateSession}
          onCancel={handleCancelNewSession}
        />
      )}
    </aside>
  )
}

// New Session Modal Component
interface NewSessionModalProps {
  providers: Array<{ id: string; name: string; models: Array<{ id: string; name: string }> }>
  defaultProvider: string
  defaultModel: string
  projectPath: string
  onSubmit: (params: CreateSessionParams) => void
  onCancel: () => void
}

const NewSessionModal: React.FC<NewSessionModalProps> = ({
  providers,
  defaultProvider,
  defaultModel,
  projectPath,
  onSubmit,
  onCancel,
}) => {
  const [selectedProvider, setSelectedProvider] = useState(defaultProvider)
  const [selectedModel, setSelectedModel] = useState(defaultModel)
  const [title, setTitle] = useState('')

  const currentProvider = providers.find(p => p.id === selectedProvider)
  const availableModels = currentProvider?.models || []

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault()
    onSubmit({
      projectPath,
      provider: selectedProvider,
      model: selectedModel,
      title: title.trim() || '',
    })
  }, [projectPath, selectedProvider, selectedModel, title, onSubmit])

  return (
    <div className="new-session-modal__overlay" style={modalStyles.overlay} onClick={onCancel}>
      <div className="new-session-modal" style={modalStyles.modal} onClick={e => e.stopPropagation()}>
        <h3 style={modalStyles.title}>New Session</h3>

        <form onSubmit={handleSubmit}>
          {/* Title input */}
          <div style={modalStyles.field}>
            <label style={modalStyles.label}>Title (optional)</label>
            <input
              type="text"
              value={title}
              onChange={e => setTitle(e.target.value)}
              placeholder="Enter session title..."
              style={modalStyles.input}
            />
          </div>

          {/* Project display */}
          <div style={modalStyles.field}>
            <label style={modalStyles.label}>Project Path</label>
            <div style={modalStyles.projectPathDisplay}>
              {projectPath || 'No project selected (will use default)'}
            </div>
          </div>

          {/* Provider select */}
          {providers.length > 0 && (
            <div style={modalStyles.field}>
              <label style={modalStyles.label}>Provider</label>
              <select
                value={selectedProvider}
                onChange={e => {
                  setSelectedProvider(e.target.value)
                  const newProvider = providers.find(p => p.id === e.target.value)
                  if (newProvider && newProvider.models.length > 0) {
                    setSelectedModel(newProvider.models[0].id)
                  }
                }}
                style={modalStyles.select}
              >
                {providers.map(provider => (
                  <option key={provider.id} value={provider.id}>
                    {provider.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Model select */}
          {availableModels.length > 0 && (
            <div style={modalStyles.field}>
              <label style={modalStyles.label}>Model</label>
              <select
                value={selectedModel}
                onChange={e => setSelectedModel(e.target.value)}
                style={modalStyles.select}
              >
                {availableModels.map(model => (
                  <option key={model.id} value={model.id}>
                    {model.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Actions */}
          <div style={modalStyles.actions}>
            <button
              type="button"
              onClick={onCancel}
              style={modalStyles.cancelButton}
            >
              Cancel
            </button>
            <button
              type="submit"
              style={modalStyles.submitButton}
            >
              Create Session
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    backgroundColor: '#161b22',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 16px',
    borderBottom: '1px solid #30363d',
  },
  title: {
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
    margin: 0,
  },
  filters: {
    padding: '12px',
    borderBottom: '1px solid #30363d',
  },
  list: {
    flex: 1,
    overflow: 'auto',
  },
}

const modalStyles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: 'rgba(0, 0, 0, 0.5)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
  },
  modal: {
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
    padding: '24px',
    width: '400px',
    maxWidth: '90vw',
  },
  title: {
    fontSize: '18px',
    fontWeight: 600,
    color: '#c9d1d9',
    margin: '0 0 20px 0',
  },
  field: {
    marginBottom: '16px',
  },
  label: {
    display: 'block',
    fontSize: '12px',
    fontWeight: 500,
    color: '#8b949e',
    marginBottom: '6px',
  },
  input: {
    width: '100%',
    padding: '8px 12px',
    fontSize: '14px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    outline: 'none',
    boxSizing: 'border-box',
  },
  select: {
    width: '100%',
    padding: '8px 12px',
    fontSize: '14px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    outline: 'none',
    cursor: 'pointer',
  },
  projectPathDisplay: {
    padding: '8px 12px',
    fontSize: '13px',
    color: '#8b949e',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    wordBreak: 'break-all',
  },
  actions: {
    display: 'flex',
    justifyContent: 'flex-end',
    gap: '12px',
    marginTop: '24px',
  },
  cancelButton: {
    padding: '8px 16px',
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '6px',
    cursor: 'pointer',
  },
  submitButton: {
    padding: '8px 16px',
    fontSize: '14px',
    fontWeight: 500,
    color: '#ffffff',
    backgroundColor: '#238636',
    border: 'none',
    borderRadius: '6px',
    cursor: 'pointer',
  },
}

export default SessionSidebar
