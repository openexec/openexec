/**
 * SessionForkDialog Component
 * Modal dialog for forking a session at a specific message point.
 * Allows customization of fork options including title, provider override,
 * and what content to copy.
 *
 * @module components/chat/session/SessionForkDialog
 */
import React, { useState, useCallback, useEffect } from 'react'
import type { Session, Message, ProviderInfo, ModelInfo } from '../../../types/chat'

export interface ForkOptions {
  /** Custom title for the forked session */
  title: string
  /** Override provider (optional) */
  provider: string
  /** Override model (optional) */
  model: string
  /** Copy messages up to fork point */
  copyMessages: boolean
  /** Copy tool calls (requires copyMessages) */
  copyToolCalls: boolean
  /** Copy summaries (requires copyMessages) */
  copySummaries: boolean
}

export interface ForkResult {
  /** ID of the newly forked session */
  forkedSessionId: string
  /** ID of the parent session */
  parentSessionId: string
  /** Message ID where fork occurred */
  forkPointMessageId: string
  /** Title of the forked session */
  title: string
  /** Provider of the forked session */
  provider: string
  /** Model of the forked session */
  model: string
  /** Number of messages copied */
  messagesCopied: number
  /** Number of tool calls copied */
  toolCallsCopied: number
  /** Number of summaries copied */
  summariesCopied: number
  /** Fork depth in the tree */
  forkDepth: number
  /** Chain of ancestor session IDs */
  ancestorChain: string[]
}

export interface SessionForkDialogProps {
  /** The session being forked */
  session: Session
  /** Optional message to fork at (defaults to latest) */
  forkPointMessage?: Message
  /** List of messages for selection (if no forkPointMessage provided) */
  messages?: Message[]
  /** Available providers for override selection */
  providers?: ProviderInfo[]
  /** Whether the dialog is open */
  isOpen: boolean
  /** Callback when dialog is closed */
  onClose: () => void
  /** Callback when fork is confirmed */
  onFork: (sessionId: string, forkPointMessageId: string, options: ForkOptions) => Promise<ForkResult>
  /** Callback when navigating to the forked session */
  onNavigateToSession?: (sessionId: string) => void
  /** Whether fork is in progress */
  isLoading?: boolean
}

const SessionForkDialog: React.FC<SessionForkDialogProps> = ({
  session,
  forkPointMessage,
  messages = [],
  providers = [],
  isOpen,
  onClose,
  onFork,
  onNavigateToSession,
  isLoading = false,
}) => {
  // Form state
  const [selectedMessageId, setSelectedMessageId] = useState<string>('')
  const [title, setTitle] = useState<string>('')
  const [overrideProvider, setOverrideProvider] = useState<string>('')
  const [overrideModel, setOverrideModel] = useState<string>('')
  const [copyMessages, setCopyMessages] = useState<boolean>(true)
  const [copyToolCalls, setCopyToolCalls] = useState<boolean>(true)
  const [copySummaries, setCopySummaries] = useState<boolean>(true)

  // Result state
  const [forkResult, setForkResult] = useState<ForkResult | null>(null)
  const [error, setError] = useState<string | null>(null)

  // Initialize state when dialog opens
  useEffect(() => {
    if (isOpen) {
      if (forkPointMessage) {
        setSelectedMessageId(forkPointMessage.id)
        setTitle(`Fork of "${session.title}" at message`)
      } else if (messages.length > 0) {
        setSelectedMessageId(messages[messages.length - 1].id)
        setTitle(`Fork of "${session.title}"`)
      } else {
        setTitle(`Fork of "${session.title}"`)
      }
      setOverrideProvider('')
      setOverrideModel('')
      setCopyMessages(true)
      setCopyToolCalls(true)
      setCopySummaries(true)
      setForkResult(null)
      setError(null)
    }
  }, [isOpen, forkPointMessage, messages, session.title])

  // Get available models for selected provider
  const getAvailableModels = (): ModelInfo[] => {
    if (!overrideProvider) return []
    const provider = providers.find((p) => p.id === overrideProvider)
    return provider?.models || []
  }

  // Handle fork submission
  const handleFork = useCallback(async () => {
    if (!selectedMessageId && !forkPointMessage) {
      setError('Please select a message to fork at')
      return
    }

    const messageId = forkPointMessage?.id || selectedMessageId

    try {
      setError(null)
      const options: ForkOptions = {
        title: title || `Fork of "${session.title}"`,
        provider: overrideProvider,
        model: overrideModel,
        copyMessages,
        copyToolCalls: copyMessages && copyToolCalls,
        copySummaries: copyMessages && copySummaries,
      }

      const result = await onFork(session.id, messageId, options)
      setForkResult(result)
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to fork session'
      setError(message)
    }
  }, [
    selectedMessageId,
    forkPointMessage,
    title,
    overrideProvider,
    overrideModel,
    copyMessages,
    copyToolCalls,
    copySummaries,
    session.id,
    session.title,
    onFork,
  ])

  // Handle navigation to forked session
  const handleNavigate = useCallback(() => {
    if (forkResult && onNavigateToSession) {
      onNavigateToSession(forkResult.forkedSessionId)
      onClose()
    }
  }, [forkResult, onNavigateToSession, onClose])

  // Handle backdrop click
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget && !isLoading) {
        onClose()
      }
    },
    [isLoading, onClose]
  )

  // Handle escape key
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Escape' && !isLoading) {
        onClose()
      }
    },
    [isLoading, onClose]
  )

  // Format message preview for selection
  const formatMessagePreview = (msg: Message): string => {
    const preview = msg.content.slice(0, 50)
    return msg.content.length > 50 ? `${preview}...` : preview
  }

  // Format timestamp
  const formatTimestamp = (timestamp: string): string => {
    return new Date(timestamp).toLocaleString()
  }

  if (!isOpen) return null

  // Render success state
  if (forkResult) {
    return (
      <div
        className="session-fork-dialog__backdrop"
        style={styles.backdrop}
        onClick={handleBackdropClick}
        onKeyDown={handleKeyDown}
        role="dialog"
        aria-modal="true"
        aria-labelledby="fork-dialog-title"
        tabIndex={-1}
      >
        <div className="session-fork-dialog" style={styles.dialog}>
          {/* Header */}
          <div className="session-fork-dialog__header" style={styles.header}>
            <div style={styles.headerContent}>
              <SuccessIcon />
              <h2 id="fork-dialog-title" style={styles.title}>
                Session Forked Successfully
              </h2>
            </div>
            <button
              className="session-fork-dialog__close"
              style={styles.closeButton}
              onClick={onClose}
              aria-label="Close dialog"
            >
              <CloseIcon />
            </button>
          </div>

          {/* Body - Success */}
          <div className="session-fork-dialog__body" style={styles.body}>
            <div className="session-fork-dialog__success" style={styles.successBanner}>
              <CheckCircleIcon />
              <span>
                Created new session "{forkResult.title}" as a fork of the current conversation.
              </span>
            </div>

            <div className="session-fork-dialog__details" style={styles.details}>
              {/* Fork Info */}
              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Forked Session ID</label>
                <code style={styles.code}>{forkResult.forkedSessionId}</code>
              </div>

              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Fork Depth</label>
                <span style={styles.value}>
                  Level {forkResult.forkDepth}
                  {forkResult.forkDepth > 0 && (
                    <span style={styles.dimmed}> in fork tree</span>
                  )}
                </span>
              </div>

              {/* Copied Items */}
              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Content Copied</label>
                <div style={styles.copiedStats}>
                  <span style={styles.statItem}>
                    <MessageIcon />
                    {forkResult.messagesCopied} messages
                  </span>
                  <span style={styles.statItem}>
                    <ToolIcon />
                    {forkResult.toolCallsCopied} tool calls
                  </span>
                  <span style={styles.statItem}>
                    <SummaryIcon />
                    {forkResult.summariesCopied} summaries
                  </span>
                </div>
              </div>

              {/* Provider/Model */}
              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Configuration</label>
                <span style={styles.value}>
                  {forkResult.provider} / {forkResult.model}
                </span>
              </div>

              {/* Ancestor Chain */}
              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Ancestor Chain</label>
                <div style={styles.ancestorChain}>
                  {forkResult.ancestorChain.length > 0 ? (
                    forkResult.ancestorChain.map((ancestorId, index) => (
                      <span key={ancestorId} style={styles.ancestorItem}>
                        {index > 0 && <span style={styles.ancestorArrow}>→</span>}
                        <code style={styles.ancestorId}>{ancestorId.slice(0, 8)}...</code>
                      </span>
                    ))
                  ) : (
                    <span style={styles.dimmed}>None</span>
                  )}
                  <span style={styles.ancestorArrow}>→</span>
                  <code style={{ ...styles.ancestorId, color: '#58a6ff' }}>
                    {forkResult.forkedSessionId.slice(0, 8)}... (new)
                  </code>
                </div>
              </div>
            </div>
          </div>

          {/* Footer - Success */}
          <div className="session-fork-dialog__footer" style={styles.footer}>
            <button
              className="session-fork-dialog__button"
              style={{ ...styles.button, ...styles.secondaryButton }}
              onClick={onClose}
            >
              Stay Here
            </button>
            {onNavigateToSession && (
              <button
                className="session-fork-dialog__button"
                style={{ ...styles.button, ...styles.primaryButton }}
                onClick={handleNavigate}
              >
                Open Forked Session
              </button>
            )}
          </div>
        </div>
      </div>
    )
  }

  // Render form state
  return (
    <div
      className="session-fork-dialog__backdrop"
      style={styles.backdrop}
      onClick={handleBackdropClick}
      onKeyDown={handleKeyDown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="fork-dialog-title"
      tabIndex={-1}
    >
      <div className="session-fork-dialog" style={styles.dialog}>
        {/* Header */}
        <div className="session-fork-dialog__header" style={styles.header}>
          <div style={styles.headerContent}>
            <ForkIcon />
            <h2 id="fork-dialog-title" style={styles.title}>
              Fork Session
            </h2>
          </div>
          <button
            className="session-fork-dialog__close"
            style={styles.closeButton}
            onClick={onClose}
            disabled={isLoading}
            aria-label="Close dialog"
          >
            <CloseIcon />
          </button>
        </div>

        {/* Body */}
        <div className="session-fork-dialog__body" style={styles.body}>
          {/* Info banner */}
          <div className="session-fork-dialog__info" style={styles.infoBanner}>
            <InfoIcon />
            <span>
              Forking creates a new session that branches off from this conversation.
              You can continue in a new direction while preserving the original.
            </span>
          </div>

          {/* Error banner */}
          {error && (
            <div className="session-fork-dialog__error" style={styles.errorBanner}>
              <WarningIcon />
              <span>{error}</span>
            </div>
          )}

          <div className="session-fork-dialog__form" style={styles.form}>
            {/* Parent session info */}
            <div className="session-fork-dialog__field" style={styles.field}>
              <label style={styles.label}>Parent Session</label>
              <div style={styles.parentInfo}>
                <span style={styles.parentTitle}>{session.title}</span>
                <span style={styles.parentMeta}>
                  {session.provider} / {session.model}
                </span>
              </div>
            </div>

            {/* Fork point selection */}
            {!forkPointMessage && messages.length > 0 && (
              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Fork Point</label>
                <select
                  style={styles.select}
                  value={selectedMessageId}
                  onChange={(e) => setSelectedMessageId(e.target.value)}
                  disabled={isLoading}
                >
                  <option value="">Select a message...</option>
                  {messages.map((msg) => (
                    <option key={msg.id} value={msg.id}>
                      [{msg.role}] {formatMessagePreview(msg)}
                    </option>
                  ))}
                </select>
                <span style={styles.hint}>
                  The fork will include all messages up to and including this point.
                </span>
              </div>
            )}

            {/* Fixed fork point display */}
            {forkPointMessage && (
              <div className="session-fork-dialog__field" style={styles.field}>
                <label style={styles.label}>Fork Point</label>
                <div style={styles.fixedForkPoint}>
                  <span style={styles.forkPointRole}>{forkPointMessage.role}</span>
                  <span style={styles.forkPointPreview}>
                    {formatMessagePreview(forkPointMessage)}
                  </span>
                  <span style={styles.forkPointTime}>
                    {formatTimestamp(forkPointMessage.createdAt)}
                  </span>
                </div>
              </div>
            )}

            {/* Title */}
            <div className="session-fork-dialog__field" style={styles.field}>
              <label style={styles.label}>Fork Title</label>
              <input
                type="text"
                style={styles.input}
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder={`Fork of "${session.title}"`}
                disabled={isLoading}
              />
            </div>

            {/* Provider override */}
            {providers.length > 0 && (
              <>
                <div className="session-fork-dialog__field" style={styles.field}>
                  <label style={styles.label}>Provider Override (optional)</label>
                  <select
                    style={styles.select}
                    value={overrideProvider}
                    onChange={(e) => {
                      setOverrideProvider(e.target.value)
                      setOverrideModel('')
                    }}
                    disabled={isLoading}
                  >
                    <option value="">Keep current ({session.provider})</option>
                    {providers
                      .filter((p) => p.isAvailable)
                      .map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                  </select>
                </div>

                {/* Model override */}
                {overrideProvider && (
                  <div className="session-fork-dialog__field" style={styles.field}>
                    <label style={styles.label}>Model</label>
                    <select
                      style={styles.select}
                      value={overrideModel}
                      onChange={(e) => setOverrideModel(e.target.value)}
                      disabled={isLoading}
                    >
                      <option value="">Select a model...</option>
                      {getAvailableModels().map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
              </>
            )}

            {/* Copy options */}
            <div className="session-fork-dialog__field" style={styles.field}>
              <label style={styles.label}>Copy Options</label>
              <div style={styles.checkboxGroup}>
                <label style={styles.checkboxLabel}>
                  <input
                    type="checkbox"
                    style={styles.checkbox}
                    checked={copyMessages}
                    onChange={(e) => setCopyMessages(e.target.checked)}
                    disabled={isLoading}
                  />
                  <span>Copy messages to fork</span>
                </label>
                <label
                  style={{
                    ...styles.checkboxLabel,
                    ...(copyMessages ? {} : styles.checkboxDisabled),
                  }}
                >
                  <input
                    type="checkbox"
                    style={styles.checkbox}
                    checked={copyMessages && copyToolCalls}
                    onChange={(e) => setCopyToolCalls(e.target.checked)}
                    disabled={isLoading || !copyMessages}
                  />
                  <span>Include tool calls</span>
                </label>
                <label
                  style={{
                    ...styles.checkboxLabel,
                    ...(copyMessages ? {} : styles.checkboxDisabled),
                  }}
                >
                  <input
                    type="checkbox"
                    style={styles.checkbox}
                    checked={copyMessages && copySummaries}
                    onChange={(e) => setCopySummaries(e.target.checked)}
                    disabled={isLoading || !copyMessages}
                  />
                  <span>Include summaries</span>
                </label>
              </div>
              <span style={styles.hint}>
                {copyMessages
                  ? 'Messages will be copied to the new session.'
                  : 'Fork will start with an empty conversation history.'}
              </span>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="session-fork-dialog__footer" style={styles.footer}>
          <button
            className="session-fork-dialog__button"
            style={{ ...styles.button, ...styles.cancelButton }}
            onClick={onClose}
            disabled={isLoading}
          >
            Cancel
          </button>
          <button
            className="session-fork-dialog__button"
            style={{ ...styles.button, ...styles.primaryButton }}
            onClick={handleFork}
            disabled={isLoading || (!forkPointMessage && !selectedMessageId)}
          >
            {isLoading ? (
              <>
                <LoadingSpinner />
                <span>Forking...</span>
              </>
            ) : (
              <>
                <ForkIcon />
                <span>Create Fork</span>
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}

// Icon components
const ForkIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="18" r="3" />
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="6" r="3" />
    <path d="M18 9a9 9 0 01-9 9" />
    <path d="M6 9a9 9 0 009 9" />
  </svg>
)

const CloseIcon: React.FC = () => (
  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const InfoIcon: React.FC = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="16" x2="12" y2="12" />
    <line x1="12" y1="8" x2="12.01" y2="8" />
  </svg>
)

const WarningIcon: React.FC = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
    <line x1="12" y1="9" x2="12" y2="13" />
    <line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
)

const SuccessIcon: React.FC = () => (
  <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 11.08V12a10 10 0 11-5.93-9.14" />
    <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
)

const CheckCircleIcon: React.FC = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 11.08V12a10 10 0 11-5.93-9.14" />
    <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
)

const MessageIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />
  </svg>
)

const ToolIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14.7 6.3a1 1 0 000 1.4l1.6 1.6a1 1 0 001.4 0l3.77-3.77a6 6 0 01-7.94 7.94l-6.91 6.91a2.12 2.12 0 01-3-3l6.91-6.91a6 6 0 017.94-7.94l-3.76 3.76z" />
  </svg>
)

const SummaryIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
    <polyline points="14 2 14 8 20 8" />
    <line x1="16" y1="13" x2="8" y2="13" />
    <line x1="16" y1="17" x2="8" y2="17" />
    <polyline points="10 9 9 9 8 9" />
  </svg>
)

const LoadingSpinner: React.FC = () => (
  <svg
    width="16"
    height="16"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{ animation: 'spin 1s linear infinite' }}
  >
    <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  backdrop: {
    position: 'fixed',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: 'rgba(0, 0, 0, 0.6)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
    padding: '20px',
  },
  dialog: {
    backgroundColor: '#161b22',
    borderRadius: '12px',
    border: '1px solid #30363d',
    width: '100%',
    maxWidth: '560px',
    maxHeight: '90vh',
    display: 'flex',
    flexDirection: 'column',
    overflow: 'hidden',
    boxShadow: '0 8px 32px rgba(0, 0, 0, 0.4)',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '16px 20px',
    borderBottom: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  headerContent: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    color: '#a371f7',
  },
  title: {
    margin: 0,
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  closeButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '32px',
    height: '32px',
    border: 'none',
    borderRadius: '6px',
    backgroundColor: 'transparent',
    color: '#8b949e',
    cursor: 'pointer',
    transition: 'background-color 0.2s',
  },
  body: {
    padding: '20px',
    overflowY: 'auto',
    flex: 1,
  },
  infoBanner: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '12px',
    padding: '12px 16px',
    backgroundColor: 'rgba(88, 166, 255, 0.1)',
    border: '1px solid rgba(88, 166, 255, 0.4)',
    borderRadius: '8px',
    marginBottom: '20px',
    fontSize: '13px',
    color: '#58a6ff',
    lineHeight: 1.5,
  },
  errorBanner: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '12px',
    padding: '12px 16px',
    backgroundColor: 'rgba(218, 54, 51, 0.1)',
    border: '1px solid rgba(218, 54, 51, 0.4)',
    borderRadius: '8px',
    marginBottom: '20px',
    fontSize: '13px',
    color: '#da3633',
    lineHeight: 1.5,
  },
  successBanner: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '12px',
    padding: '12px 16px',
    backgroundColor: 'rgba(35, 134, 54, 0.1)',
    border: '1px solid rgba(35, 134, 54, 0.4)',
    borderRadius: '8px',
    marginBottom: '20px',
    fontSize: '13px',
    color: '#238636',
    lineHeight: 1.5,
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  details: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  field: {
    display: 'flex',
    flexDirection: 'column',
    gap: '6px',
  },
  label: {
    fontSize: '11px',
    fontWeight: 500,
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    color: '#8b949e',
  },
  value: {
    fontSize: '14px',
    color: '#c9d1d9',
  },
  dimmed: {
    color: '#8b949e',
  },
  input: {
    width: '100%',
    padding: '10px 12px',
    fontSize: '14px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    fontFamily: 'inherit',
  },
  select: {
    width: '100%',
    padding: '10px 12px',
    fontSize: '14px',
    color: '#c9d1d9',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    fontFamily: 'inherit',
    cursor: 'pointer',
  },
  hint: {
    fontSize: '12px',
    color: '#6e7681',
    lineHeight: 1.4,
  },
  code: {
    fontSize: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#58a6ff',
    backgroundColor: '#0d1117',
    padding: '4px 8px',
    borderRadius: '4px',
    display: 'inline-block',
  },
  parentInfo: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  parentTitle: {
    fontSize: '14px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  parentMeta: {
    fontSize: '12px',
    color: '#8b949e',
  },
  fixedForkPoint: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    padding: '10px 12px',
    backgroundColor: '#0d1117',
    borderRadius: '6px',
    border: '1px solid #30363d',
  },
  forkPointRole: {
    fontSize: '11px',
    fontWeight: 500,
    textTransform: 'uppercase',
    color: '#a371f7',
  },
  forkPointPreview: {
    fontSize: '13px',
    color: '#c9d1d9',
    lineHeight: 1.4,
  },
  forkPointTime: {
    fontSize: '11px',
    color: '#6e7681',
  },
  checkboxGroup: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  checkboxLabel: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontSize: '13px',
    color: '#c9d1d9',
    cursor: 'pointer',
  },
  checkboxDisabled: {
    color: '#6e7681',
    cursor: 'not-allowed',
  },
  checkbox: {
    width: '16px',
    height: '16px',
    accentColor: '#58a6ff',
  },
  copiedStats: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '12px',
  },
  statItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '13px',
    color: '#c9d1d9',
    backgroundColor: '#21262d',
    padding: '4px 10px',
    borderRadius: '4px',
  },
  ancestorChain: {
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    gap: '4px',
  },
  ancestorItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  ancestorArrow: {
    color: '#6e7681',
    fontSize: '12px',
  },
  ancestorId: {
    fontSize: '11px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#8b949e',
    backgroundColor: '#0d1117',
    padding: '2px 6px',
    borderRadius: '3px',
  },
  footer: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-end',
    gap: '12px',
    padding: '16px 20px',
    borderTop: '1px solid #30363d',
    backgroundColor: '#0d1117',
  },
  button: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '8px',
    padding: '8px 16px',
    fontSize: '14px',
    fontWeight: 500,
    borderRadius: '6px',
    border: 'none',
    cursor: 'pointer',
    transition: 'background-color 0.2s, opacity 0.2s',
  },
  cancelButton: {
    backgroundColor: '#21262d',
    color: '#c9d1d9',
    border: '1px solid #30363d',
  },
  secondaryButton: {
    backgroundColor: '#21262d',
    color: '#c9d1d9',
    border: '1px solid #30363d',
  },
  primaryButton: {
    backgroundColor: '#a371f7',
    color: '#ffffff',
  },
}

export default SessionForkDialog
