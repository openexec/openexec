/**
 * ForkAncestryTree Component
 *
 * Visualizes the fork ancestry chain for a session.
 * Shows the hierarchical relationship between parent and child sessions.
 *
 * @module components/chat/session/ForkAncestryTree
 */

import React from 'react'

export interface AncestorSession {
  /** Session ID */
  id: string
  /** Session title */
  title: string
  /** Whether this is the current session */
  isCurrent?: boolean
  /** Whether this is the root session */
  isRoot?: boolean
}

export interface ForkAncestryTreeProps {
  /** Chain of ancestor sessions from root to current */
  ancestors: AncestorSession[]
  /** Current session ID */
  currentSessionId: string
  /** Fork depth of the current session */
  forkDepth: number
  /** Callback when an ancestor session is clicked */
  onNavigateToSession?: (sessionId: string) => void
}

/**
 * ForkAncestryTree Component
 *
 * Displays a visual representation of the fork ancestry chain,
 * allowing navigation to parent sessions.
 */
const ForkAncestryTree: React.FC<ForkAncestryTreeProps> = ({
  ancestors,
  currentSessionId,
  forkDepth,
  onNavigateToSession,
}) => {
  if (ancestors.length === 0) {
    return (
      <div className="fork-ancestry-tree" style={styles.container}>
        <div style={styles.rootIndicator}>
          <RootIcon />
          <span style={styles.rootText}>Root Session</span>
        </div>
      </div>
    )
  }

  return (
    <div className="fork-ancestry-tree" style={styles.container}>
      <div className="fork-ancestry-tree__header" style={styles.header}>
        <ForkIcon />
        <span style={styles.headerText}>Fork Ancestry (Level {forkDepth})</span>
      </div>

      <div className="fork-ancestry-tree__chain" style={styles.chain}>
        {ancestors.map((ancestor, index) => {
          const isLast = index === ancestors.length - 1
          const isCurrent = ancestor.id === currentSessionId

          return (
            <div key={ancestor.id} className="fork-ancestry-tree__node" style={styles.nodeContainer}>
              {/* Connector line */}
              {index > 0 && (
                <div className="fork-ancestry-tree__connector" style={styles.connector}>
                  <div style={styles.connectorLine} />
                </div>
              )}

              {/* Node */}
              <button
                className={`fork-ancestry-tree__session ${isCurrent ? 'fork-ancestry-tree__session--current' : ''}`}
                style={{
                  ...styles.node,
                  ...(isCurrent ? styles.nodeCurrent : {}),
                  ...(ancestor.isRoot ? styles.nodeRoot : {}),
                }}
                onClick={() => onNavigateToSession?.(ancestor.id)}
                disabled={isCurrent || !onNavigateToSession}
                title={isCurrent ? 'Current session' : `Navigate to "${ancestor.title}"`}
              >
                {/* Node indicator */}
                <div
                  style={{
                    ...styles.nodeIndicator,
                    ...(ancestor.isRoot ? styles.nodeIndicatorRoot : {}),
                    ...(isCurrent ? styles.nodeIndicatorCurrent : {}),
                  }}
                >
                  {ancestor.isRoot ? (
                    <RootIcon />
                  ) : isCurrent ? (
                    <CurrentIcon />
                  ) : (
                    <BranchIcon />
                  )}
                </div>

                {/* Session info */}
                <div style={styles.nodeContent}>
                  <span style={styles.nodeTitle}>
                    {ancestor.title || 'Untitled Session'}
                  </span>
                  <span style={styles.nodeLevel}>
                    {ancestor.isRoot ? 'Root' : `Level ${index}`}
                  </span>
                </div>

                {/* Navigation arrow */}
                {!isCurrent && onNavigateToSession && (
                  <div style={styles.nodeArrow}>
                    <ArrowIcon />
                  </div>
                )}
              </button>

              {/* Branch line for non-last items */}
              {!isLast && (
                <div style={styles.branchLine} />
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    padding: '12px',
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    color: '#a371f7',
  },
  headerText: {
    fontSize: '12px',
    fontWeight: 600,
  },
  chain: {
    display: 'flex',
    flexDirection: 'column',
    gap: '0',
    paddingLeft: '4px',
  },
  nodeContainer: {
    display: 'flex',
    flexDirection: 'column',
    position: 'relative',
  },
  connector: {
    display: 'flex',
    alignItems: 'center',
    height: '16px',
    paddingLeft: '11px',
  },
  connectorLine: {
    width: '2px',
    height: '100%',
    backgroundColor: '#30363d',
  },
  node: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
    padding: '8px 12px',
    backgroundColor: '#0d1117',
    border: '1px solid #30363d',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'all 0.15s ease',
    textAlign: 'left',
  },
  nodeCurrent: {
    backgroundColor: 'rgba(163, 113, 247, 0.1)',
    borderColor: '#a371f7',
    cursor: 'default',
  },
  nodeRoot: {
    backgroundColor: 'rgba(88, 166, 255, 0.1)',
    borderColor: '#58a6ff',
  },
  nodeIndicator: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '24px',
    height: '24px',
    borderRadius: '50%',
    backgroundColor: '#21262d',
    color: '#8b949e',
    flexShrink: 0,
  },
  nodeIndicatorRoot: {
    backgroundColor: 'rgba(88, 166, 255, 0.2)',
    color: '#58a6ff',
  },
  nodeIndicatorCurrent: {
    backgroundColor: 'rgba(163, 113, 247, 0.2)',
    color: '#a371f7',
  },
  nodeContent: {
    display: 'flex',
    flexDirection: 'column',
    flex: 1,
    minWidth: 0,
  },
  nodeTitle: {
    fontSize: '13px',
    fontWeight: 500,
    color: '#c9d1d9',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  nodeLevel: {
    fontSize: '11px',
    color: '#8b949e',
  },
  nodeArrow: {
    color: '#8b949e',
    flexShrink: 0,
  },
  branchLine: {
    position: 'absolute',
    left: '23px',
    bottom: '-8px',
    width: '2px',
    height: '16px',
    backgroundColor: '#30363d',
  },
  rootIndicator: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    color: '#58a6ff',
    padding: '8px',
  },
  rootText: {
    fontSize: '12px',
    fontWeight: 500,
  },
}

// Icon components
const ForkIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="18" r="3" />
    <circle cx="6" cy="6" r="3" />
    <circle cx="18" cy="6" r="3" />
    <path d="M18 9a9 9 0 01-9 9" />
    <path d="M6 9a9 9 0 009 9" />
  </svg>
)

const RootIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <circle cx="12" cy="12" r="3" fill="currentColor" />
  </svg>
)

const BranchIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="4" />
  </svg>
)

const CurrentIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="9 12 11 14 15 10" />
  </svg>
)

const ArrowIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="9 18 15 12 9 6" />
  </svg>
)

export default ForkAncestryTree
