/**
 * StageTimeline Component
 *
 * Displays the blueprint execution stages in a visual timeline.
 * Shows status, duration, and progress for each stage.
 *
 * @module components/chat/blueprint/StageTimeline
 */

import React, { useMemo } from 'react'
import type {
  Stage,
  StageName,
  StageStatus,
  BlueprintState,
} from '../../../types/blueprint'
import {
  BLUEPRINT_STAGES,
  STAGE_INFO,
  STAGE_STATUS_INFO,
} from '../../../types/blueprint'

export interface StageTimelineProps {
  /** Blueprint execution state */
  blueprintState?: BlueprintState
  /** Individual stages (alternative to blueprintState) */
  stages?: Stage[]
  /** Whether the blueprint is currently running */
  isRunning?: boolean
  /** Callback when a stage is clicked for details */
  onStageClick?: (stageName: StageName) => void
  /** Whether to show compact view */
  compact?: boolean
}

/**
 * StageTimeline Component
 *
 * Renders a horizontal or vertical timeline showing the progress
 * of blueprint execution through its stages.
 */
const StageTimeline: React.FC<StageTimelineProps> = ({
  blueprintState,
  stages: propStages,
  isRunning = false,
  onStageClick,
  compact = false,
}) => {
  // Build stages array from either prop
  const stages = useMemo((): Stage[] => {
    if (propStages) {
      return propStages
    }

    if (blueprintState?.stages) {
      return blueprintState.stages
    }

    // Return default stages with pending status
    return BLUEPRINT_STAGES.map((name) => ({
      name,
      description: STAGE_INFO[name].description,
      type: STAGE_INFO[name].type,
      status: 'pending' as StageStatus,
    }))
  }, [blueprintState, propStages])

  // Create a map for quick lookup
  const stageMap = useMemo(() => {
    const map = new Map<StageName, Stage>()
    for (const stage of stages) {
      map.set(stage.name, stage)
    }
    return map
  }, [stages])

  // Format duration for display
  const formatDuration = (ms?: number): string => {
    if (!ms) return ''
    if (ms < 1000) return `${ms}ms`
    if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
    return `${(ms / 60000).toFixed(1)}m`
  }

  // Get stage status icon
  const getStatusIcon = (status: StageStatus): React.ReactNode => {
    switch (status) {
      case 'pending':
        return <PendingIcon />
      case 'running':
        return <RunningIcon />
      case 'completed':
        return <CompletedIcon />
      case 'failed':
        return <FailedIcon />
      case 'skipped':
        return <SkippedIcon />
      default:
        return <PendingIcon />
    }
  }

  return (
    <div
      className="stage-timeline"
      style={{
        ...styles.container,
        ...(compact ? styles.containerCompact : {}),
      }}
    >
      <div className="stage-timeline__header" style={styles.header}>
        <h3 style={styles.title}>Blueprint Stages</h3>
        {isRunning && (
          <span style={styles.runningBadge}>
            <RunningIndicator />
            Running
          </span>
        )}
      </div>

      <div className="stage-timeline__stages" style={styles.stages}>
        {BLUEPRINT_STAGES.map((stageName, index) => {
          const stage = stageMap.get(stageName) || {
            name: stageName,
            description: STAGE_INFO[stageName].description,
            type: STAGE_INFO[stageName].type,
            status: 'pending' as StageStatus,
          }
          const info = STAGE_INFO[stageName]
          const statusInfo = STAGE_STATUS_INFO[stage.status]
          const isLast = index === BLUEPRINT_STAGES.length - 1

          return (
            <React.Fragment key={stageName}>
              <div
                className={`stage-timeline__stage stage-timeline__stage--${stage.status}`}
                style={{
                  ...styles.stage,
                  cursor: onStageClick ? 'pointer' : 'default',
                }}
                onClick={() => onStageClick?.(stageName)}
                role={onStageClick ? 'button' : undefined}
                tabIndex={onStageClick ? 0 : undefined}
                onKeyDown={
                  onStageClick
                    ? (e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          onStageClick(stageName)
                        }
                      }
                    : undefined
                }
              >
                {/* Status indicator */}
                <div
                  className="stage-timeline__status"
                  style={{
                    ...styles.statusIcon,
                    color: statusInfo.color,
                    borderColor: statusInfo.color,
                    backgroundColor:
                      stage.status === 'running' ? `${statusInfo.color}20` : 'transparent',
                  }}
                >
                  {getStatusIcon(stage.status)}
                </div>

                {/* Stage info */}
                <div className="stage-timeline__info" style={styles.stageInfo}>
                  <div className="stage-timeline__name" style={styles.stageName}>
                    {info.label}
                    {stage.type === 'agentic' && (
                      <span style={styles.agenticBadge} title="Requires LLM reasoning">
                        AI
                      </span>
                    )}
                  </div>

                  {!compact && (
                    <div className="stage-timeline__description" style={styles.stageDescription}>
                      {info.description}
                    </div>
                  )}

                  {/* Duration and attempt info */}
                  <div className="stage-timeline__meta" style={styles.stageMeta}>
                    {stage.durationMs !== undefined && stage.durationMs > 0 && (
                      <span style={styles.metaItem}>
                        <ClockIcon />
                        {formatDuration(stage.durationMs)}
                      </span>
                    )}
                    {stage.attempt !== undefined && stage.attempt > 1 && (
                      <span style={{ ...styles.metaItem, color: '#f0883e' }}>
                        <RetryIcon />
                        Attempt {stage.attempt}
                        {stage.maxRetries ? ` / ${stage.maxRetries}` : ''}
                      </span>
                    )}
                    {stage.error && (
                      <span style={{ ...styles.metaItem, color: '#da3633' }} title={stage.error}>
                        <ErrorIcon />
                        Error
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {/* Connector line */}
              {!isLast && (
                <div
                  className="stage-timeline__connector"
                  style={{
                    ...styles.connector,
                    backgroundColor:
                      stage.status === 'completed'
                        ? STAGE_STATUS_INFO.completed.color
                        : '#30363d',
                  }}
                />
              )}
            </React.Fragment>
          )
        })}
      </div>

      {/* Blueprint summary */}
      {blueprintState && (
        <div className="stage-timeline__summary" style={styles.summary}>
          <span style={styles.summaryItem}>
            ID: {blueprintState.id.slice(0, 8)}...
          </span>
          {blueprintState.totalDurationMs !== undefined && (
            <span style={styles.summaryItem}>
              Total: {formatDuration(blueprintState.totalDurationMs)}
            </span>
          )}
          <span
            style={{
              ...styles.summaryItem,
              color: blueprintState.status === 'failed' ? '#da3633' : '#238636',
            }}
          >
            {blueprintState.status.charAt(0).toUpperCase() + blueprintState.status.slice(1)}
          </span>
        </div>
      )}
    </div>
  )
}

// Icon components
const PendingIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
  </svg>
)

const RunningIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const CompletedIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
    <polyline points="22 4 12 14.01 9 11.01" />
  </svg>
)

const FailedIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="15" y1="9" x2="9" y2="15" />
    <line x1="9" y1="9" x2="15" y2="15" />
  </svg>
)

const SkippedIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="8" y1="12" x2="16" y2="12" />
  </svg>
)

const RunningIndicator: React.FC = () => (
  <svg
    width="12"
    height="12"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{ animation: 'spin 1s linear infinite' }}
  >
    <circle cx="12" cy="12" r="10" strokeDasharray="30 50" />
  </svg>
)

const ClockIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
)

const RetryIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M23 4v6h-6" />
    <path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="8" x2="12" y2="12" />
    <line x1="12" y1="16" x2="12.01" y2="16" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    backgroundColor: '#161b22',
    borderRadius: '8px',
    border: '1px solid #30363d',
    padding: '16px',
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
  },
  containerCompact: {
    padding: '12px',
    gap: '8px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  title: {
    margin: 0,
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  runningBadge: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '11px',
    fontWeight: 500,
    color: '#58a6ff',
    backgroundColor: '#58a6ff20',
    padding: '2px 8px',
    borderRadius: '12px',
  },
  stages: {
    display: 'flex',
    flexDirection: 'column',
    gap: '0',
  },
  stage: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '12px',
    padding: '8px 0',
    transition: 'background-color 0.2s',
  },
  statusIcon: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '28px',
    height: '28px',
    borderRadius: '50%',
    border: '2px solid',
    flexShrink: 0,
  },
  stageInfo: {
    flex: 1,
    minWidth: 0,
  },
  stageName: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '13px',
    fontWeight: 500,
    color: '#c9d1d9',
  },
  agenticBadge: {
    fontSize: '9px',
    fontWeight: 600,
    color: '#a371f7',
    backgroundColor: '#a371f720',
    padding: '1px 4px',
    borderRadius: '3px',
  },
  stageDescription: {
    fontSize: '12px',
    color: '#8b949e',
    marginTop: '2px',
  },
  stageMeta: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    marginTop: '4px',
    fontSize: '11px',
    color: '#8b949e',
  },
  metaItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  connector: {
    width: '2px',
    height: '8px',
    marginLeft: '13px',
    backgroundColor: '#30363d',
    transition: 'background-color 0.3s',
  },
  summary: {
    display: 'flex',
    alignItems: 'center',
    gap: '16px',
    paddingTop: '12px',
    borderTop: '1px solid #21262d',
    fontSize: '11px',
    color: '#8b949e',
  },
  summaryItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
}

export default StageTimeline
