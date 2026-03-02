/**
 * ProviderAvailabilityIndicator Component
 *
 * Displays the availability status of AI providers with:
 * - Visual indicators (colored dots) for each provider
 * - Tooltips showing status details
 * - Compact and expanded display modes
 * - Last updated timestamp
 * - Manual refresh button
 *
 * @module components/chat/header/ProviderAvailabilityIndicator
 */
import React, { useState } from 'react'
import type { ProviderInfo } from '../../../types/chat'

// =============================================================================
// Types
// =============================================================================

export interface ProviderAvailabilityIndicatorProps {
  /** List of providers with availability status */
  providers: ProviderInfo[]
  /** Whether data is currently loading */
  loading?: boolean
  /** Error message if fetch failed */
  error?: string
  /** Last time availability was checked */
  lastUpdated?: Date
  /** Callback to refresh availability */
  onRefresh?: () => void
  /** Display mode: compact shows dots only, expanded shows names */
  mode?: 'compact' | 'expanded'
  /** Show refresh button (default: true in expanded mode) */
  showRefresh?: boolean
  /** Show last updated timestamp (default: true in expanded mode) */
  showTimestamp?: boolean
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Get color for provider availability status
 */
const getStatusColor = (isAvailable: boolean): string => {
  return isAvailable ? '#238636' : '#da3633'
}

/**
 * Get provider brand color
 */
const getProviderBrandColor = (providerId: string): string => {
  const colors: Record<string, string> = {
    anthropic: '#d97757',
    openai: '#10a37f',
    gemini: '#4285f4',
    google: '#4285f4',
  }
  return colors[providerId.toLowerCase()] || '#8b949e'
}

/**
 * Format relative time for last updated
 */
const formatRelativeTime = (date: Date): string => {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)

  if (seconds < 10) return 'just now'
  if (seconds < 60) return `${seconds}s ago`

  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`

  return date.toLocaleDateString()
}

// =============================================================================
// Component
// =============================================================================

const ProviderAvailabilityIndicator: React.FC<ProviderAvailabilityIndicatorProps> = ({
  providers,
  loading = false,
  error,
  lastUpdated,
  onRefresh,
  mode = 'compact',
  showRefresh = mode === 'expanded',
  showTimestamp = mode === 'expanded',
}) => {
  const [hoveredProvider, setHoveredProvider] = useState<string | null>(null)

  const isCompact = mode === 'compact'
  const availableCount = providers.filter((p) => p.isAvailable).length
  const totalCount = providers.length

  return (
    <div className="provider-availability" style={styles.container}>
      {/* Provider Status Indicators */}
      <div className="provider-availability__indicators" style={styles.indicators}>
        {providers.map((provider) => (
          <div
            key={provider.id}
            className="provider-availability__indicator"
            style={styles.indicatorContainer}
            onMouseEnter={() => setHoveredProvider(provider.id)}
            onMouseLeave={() => setHoveredProvider(null)}
          >
            {/* Status dot with provider brand color border */}
            <span
              className="provider-availability__dot"
              style={{
                ...styles.statusDot,
                backgroundColor: getStatusColor(provider.isAvailable),
                borderColor: getProviderBrandColor(provider.id),
              }}
              title={`${provider.name}: ${provider.isAvailable ? 'Available' : provider.statusMessage || 'Unavailable'}`}
            />

            {/* Provider name (expanded mode only) */}
            {!isCompact && (
              <span
                className="provider-availability__name"
                style={{
                  ...styles.providerName,
                  color: provider.isAvailable ? '#c9d1d9' : '#8b949e',
                }}
              >
                {provider.name}
              </span>
            )}

            {/* Tooltip on hover */}
            {hoveredProvider === provider.id && (
              <div className="provider-availability__tooltip" style={styles.tooltip}>
                <div style={styles.tooltipHeader}>
                  <span style={styles.tooltipProviderName}>{provider.name}</span>
                  <span
                    style={{
                      ...styles.tooltipStatus,
                      color: provider.isAvailable ? '#3fb950' : '#f85149',
                    }}
                  >
                    {provider.isAvailable ? 'Available' : 'Unavailable'}
                  </span>
                </div>
                {!provider.isAvailable && provider.statusMessage && (
                  <div style={styles.tooltipReason}>{provider.statusMessage}</div>
                )}
                {provider.isAvailable && provider.models.length > 0 && (
                  <div style={styles.tooltipModels}>
                    {provider.models.length} model{provider.models.length !== 1 ? 's' : ''} available
                  </div>
                )}
              </div>
            )}
          </div>
        ))}
      </div>

      {/* Summary count (compact mode) */}
      {isCompact && (
        <span className="provider-availability__summary" style={styles.summary}>
          {availableCount}/{totalCount}
        </span>
      )}

      {/* Loading indicator */}
      {loading && (
        <span className="provider-availability__loading" style={styles.loading}>
          <LoadingSpinner />
        </span>
      )}

      {/* Error indicator */}
      {error && !loading && (
        <span
          className="provider-availability__error"
          style={styles.errorIndicator}
          title={error}
        >
          <ErrorIcon />
        </span>
      )}

      {/* Last updated timestamp */}
      {showTimestamp && lastUpdated && !loading && (
        <span className="provider-availability__timestamp" style={styles.timestamp}>
          Updated {formatRelativeTime(lastUpdated)}
        </span>
      )}

      {/* Refresh button */}
      {showRefresh && onRefresh && (
        <button
          className="provider-availability__refresh"
          style={{
            ...styles.refreshButton,
            ...(loading ? styles.refreshButtonDisabled : {}),
          }}
          onClick={onRefresh}
          disabled={loading}
          title="Refresh availability"
          aria-label="Refresh provider availability"
        >
          <RefreshIcon spinning={loading} />
        </button>
      )}
    </div>
  )
}

// =============================================================================
// Icon Components
// =============================================================================

const LoadingSpinner: React.FC = () => (
  <svg
    width="12"
    height="12"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{
      animation: 'spin 1s linear infinite',
    }}
  >
    <circle cx="12" cy="12" r="10" strokeOpacity="0.25" />
    <path d="M12 2a10 10 0 0 1 10 10" />
    <style>{`
      @keyframes spin {
        from { transform: rotate(0deg); }
        to { transform: rotate(360deg); }
      }
    `}</style>
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg
    width="12"
    height="12"
    viewBox="0 0 24 24"
    fill="none"
    stroke="#f85149"
    strokeWidth="2"
  >
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="8" x2="12" y2="12" />
    <line x1="12" y1="16" x2="12.01" y2="16" />
  </svg>
)

const RefreshIcon: React.FC<{ spinning?: boolean }> = ({ spinning }) => (
  <svg
    width="12"
    height="12"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{
      animation: spinning ? 'spin 1s linear infinite' : undefined,
    }}
  >
    <polyline points="23 4 23 10 17 10" />
    <polyline points="1 20 1 14 7 14" />
    <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    <style>{`
      @keyframes spin {
        from { transform: rotate(0deg); }
        to { transform: rotate(360deg); }
      }
    `}</style>
  </svg>
)

// =============================================================================
// Styles
// =============================================================================

const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    fontSize: '12px',
    color: '#8b949e',
  },
  indicators: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
  },
  indicatorContainer: {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    cursor: 'default',
  },
  statusDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    border: '2px solid',
    boxSizing: 'border-box',
    flexShrink: 0,
    transition: 'transform 0.15s ease',
  },
  providerName: {
    fontSize: '11px',
    fontWeight: 500,
  },
  summary: {
    fontSize: '11px',
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  loading: {
    display: 'flex',
    alignItems: 'center',
    color: '#8b949e',
  },
  errorIndicator: {
    display: 'flex',
    alignItems: 'center',
    cursor: 'help',
  },
  timestamp: {
    fontSize: '10px',
    color: '#6e7681',
  },
  refreshButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '4px',
    background: 'transparent',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    color: '#8b949e',
    transition: 'color 0.15s ease, background-color 0.15s ease',
  },
  refreshButtonDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
  tooltip: {
    position: 'absolute',
    top: '100%',
    left: '50%',
    transform: 'translateX(-50%)',
    marginTop: '8px',
    padding: '8px 10px',
    backgroundColor: '#1c2128',
    border: '1px solid #30363d',
    borderRadius: '6px',
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.3)',
    zIndex: 1000,
    minWidth: '120px',
    whiteSpace: 'nowrap',
  },
  tooltipHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    gap: '12px',
  },
  tooltipProviderName: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  tooltipStatus: {
    fontSize: '11px',
    fontWeight: 500,
  },
  tooltipReason: {
    marginTop: '4px',
    fontSize: '11px',
    color: '#8b949e',
  },
  tooltipModels: {
    marginTop: '4px',
    fontSize: '10px',
    color: '#6e7681',
  },
}

export default ProviderAvailabilityIndicator
