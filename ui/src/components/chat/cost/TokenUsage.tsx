/**
 * TokenUsage Component
 *
 * Displays input and output token counts with visual breakdown.
 *
 * @module components/chat/cost/TokenUsage
 */

import React from 'react'

export interface TokenUsageProps {
  /** Number of input tokens */
  inputTokens: number
  /** Number of output tokens */
  outputTokens: number
  /** Optional cache read tokens */
  cacheReadTokens?: number
  /** Optional cache write tokens */
  cacheWriteTokens?: number
  /** Maximum context window for progress bar */
  maxTokens?: number
  /** Size variant */
  size?: 'small' | 'medium' | 'large'
}

/**
 * Format token count with K/M suffix
 */
const formatTokens = (count: number): string => {
  if (count >= 1000000) {
    return `${(count / 1000000).toFixed(1)}M`
  }
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1)}K`
  }
  return count.toLocaleString()
}

const TokenUsage: React.FC<TokenUsageProps> = ({
  inputTokens,
  outputTokens,
  cacheReadTokens = 0,
  cacheWriteTokens = 0,
  maxTokens,
  size = 'medium',
}) => {
  const totalTokens = inputTokens + outputTokens
  const hasCache = cacheReadTokens > 0 || cacheWriteTokens > 0

  // Calculate progress percentage if max is provided
  const usagePercent = maxTokens ? (totalTokens / maxTokens) * 100 : 0
  const showProgress = maxTokens && usagePercent > 0

  // Get color based on usage level
  const getUsageColor = (): string => {
    if (usagePercent >= 90) return '#f85149'
    if (usagePercent >= 75) return '#ffd33d'
    return '#3fb950'
  }

  const sizeStyles = {
    small: { fontSize: '11px', gap: '8px' },
    medium: { fontSize: '12px', gap: '12px' },
    large: { fontSize: '14px', gap: '16px' },
  }

  const currentSize = sizeStyles[size]

  return (
    <div className="token-usage" style={styles.container}>
      {/* Token counts */}
      <div className="token-usage__counts" style={{ ...styles.counts, gap: currentSize.gap }}>
        {/* Input tokens */}
        <div className="token-usage__item" style={styles.item}>
          <span className="token-usage__label" style={{ ...styles.label, fontSize: currentSize.fontSize }}>
            Input
          </span>
          <span className="token-usage__value" style={{ ...styles.value, fontSize: currentSize.fontSize }}>
            {formatTokens(inputTokens)}
          </span>
          <span className="token-usage__indicator" style={{ ...styles.indicator, backgroundColor: '#79c0ff' }} />
        </div>

        {/* Output tokens */}
        <div className="token-usage__item" style={styles.item}>
          <span className="token-usage__label" style={{ ...styles.label, fontSize: currentSize.fontSize }}>
            Output
          </span>
          <span className="token-usage__value" style={{ ...styles.value, fontSize: currentSize.fontSize }}>
            {formatTokens(outputTokens)}
          </span>
          <span className="token-usage__indicator" style={{ ...styles.indicator, backgroundColor: '#7ee787' }} />
        </div>

        {/* Total tokens */}
        <div className="token-usage__item token-usage__total" style={styles.item}>
          <span className="token-usage__label" style={{ ...styles.label, fontSize: currentSize.fontSize }}>
            Total
          </span>
          <span className="token-usage__value" style={{ ...styles.totalValue, fontSize: currentSize.fontSize }}>
            {formatTokens(totalTokens)}
          </span>
        </div>
      </div>

      {/* Cache tokens (if present) */}
      {hasCache && (
        <div className="token-usage__cache" style={styles.cache}>
          {cacheReadTokens > 0 && (
            <span style={styles.cacheItem}>
              <CacheIcon /> Cache read: {formatTokens(cacheReadTokens)}
            </span>
          )}
          {cacheWriteTokens > 0 && (
            <span style={styles.cacheItem}>
              Cache write: {formatTokens(cacheWriteTokens)}
            </span>
          )}
        </div>
      )}

      {/* Context usage progress bar */}
      {showProgress && (
        <div className="token-usage__progress" style={styles.progress}>
          <div className="token-usage__progress-bar" style={styles.progressBar}>
            <div
              className="token-usage__progress-fill"
              style={{
                ...styles.progressFill,
                width: `${Math.min(usagePercent, 100)}%`,
                backgroundColor: getUsageColor(),
              }}
            />
          </div>
          <span className="token-usage__progress-label" style={styles.progressLabel}>
            {usagePercent.toFixed(1)}% of {formatTokens(maxTokens)} context
          </span>
        </div>
      )}
    </div>
  )
}

// Icon component
const CacheIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: '4px', verticalAlign: 'middle' }}>
    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  counts: {
    display: 'flex',
    alignItems: 'center',
    flexWrap: 'wrap',
  },
  item: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  label: {
    color: '#8b949e',
  },
  value: {
    color: '#c9d1d9',
    fontWeight: 500,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  totalValue: {
    color: '#c9d1d9',
    fontWeight: 600,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  indicator: {
    width: '8px',
    height: '8px',
    borderRadius: '2px',
    marginLeft: '2px',
  },
  cache: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '12px',
    fontSize: '10px',
    color: '#6e7681',
  },
  cacheItem: {
    display: 'flex',
    alignItems: 'center',
  },
  progress: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    marginTop: '4px',
  },
  progressBar: {
    height: '4px',
    backgroundColor: '#21262d',
    borderRadius: '2px',
    overflow: 'hidden',
  },
  progressFill: {
    height: '100%',
    borderRadius: '2px',
    transition: 'width 0.3s ease',
  },
  progressLabel: {
    fontSize: '10px',
    color: '#6e7681',
    textAlign: 'right',
  },
}

export default TokenUsage
