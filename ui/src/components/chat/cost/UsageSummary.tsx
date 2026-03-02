/**
 * UsageSummary Component
 *
 * Displays aggregated usage statistics including tokens, costs, requests,
 * and tool call metrics. Supports breakdown by provider and time periods.
 *
 * @module components/chat/cost/UsageSummary
 */

import React, { useMemo } from 'react'
import type { UsageSummaryData, ProviderStats } from '../../../types/chat'

export interface UsageSummaryProps {
  /** Usage summary data */
  data: UsageSummaryData
  /** Display variant */
  variant?: 'compact' | 'detailed'
  /** Show provider breakdown */
  showProviderBreakdown?: boolean
  /** Show tool call statistics */
  showToolStats?: boolean
  /** Show time period info */
  showPeriod?: boolean
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

/**
 * Format cost to USD string
 */
const formatCost = (cost: number): string => {
  if (cost === 0) return '$0.00'
  if (cost < 0.0001) return '<$0.0001'
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  return `$${cost.toFixed(2)}`
}

/**
 * Format duration in milliseconds to human readable string
 */
const formatDuration = (ms: number): string => {
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

/**
 * Format date range for period display
 */
const formatPeriod = (start?: string, end?: string): string => {
  if (!start && !end) return 'All time'
  const startDate = start ? new Date(start) : null
  const endDate = end ? new Date(end) : new Date()

  const formatDate = (d: Date): string => {
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
  }

  if (startDate && endDate) {
    return `${formatDate(startDate)} - ${formatDate(endDate)}`
  }
  if (startDate) {
    return `Since ${formatDate(startDate)}`
  }
  if (endDate) {
    return `Until ${formatDate(endDate)}`
  }
  return 'All time'
}

const UsageSummary: React.FC<UsageSummaryProps> = ({
  data,
  variant = 'detailed',
  showProviderBreakdown = true,
  showToolStats = true,
  showPeriod = true,
}) => {
  const { usage, toolCalls, periodStart, periodEnd, activeSessionCount, totalSessionCount } = data

  // Calculate derived statistics
  const stats = useMemo(() => {
    const totalTokens = usage.totalTokensInput + usage.totalTokensOutput
    const successRate = usage.totalRequests > 0
      ? (usage.successfulRequests / usage.totalRequests) * 100
      : 0
    const toolApprovalRate = toolCalls.totalRequested > 0
      ? ((toolCalls.totalApproved + toolCalls.totalAutoApproved) / toolCalls.totalRequested) * 100
      : 0
    const toolSuccessRate = toolCalls.totalCompleted + toolCalls.totalFailed > 0
      ? (toolCalls.totalCompleted / (toolCalls.totalCompleted + toolCalls.totalFailed)) * 100
      : 0

    return {
      totalTokens,
      successRate,
      toolApprovalRate,
      toolSuccessRate,
    }
  }, [usage, toolCalls])

  // Sort providers by cost (descending)
  const sortedProviders = useMemo(() => {
    if (!usage.byProvider) return []
    return Object.values(usage.byProvider).sort((a, b) => b.totalCostUsd - a.totalCostUsd)
  }, [usage.byProvider])

  // Sort tools by usage (descending)
  const sortedTools = useMemo(() => {
    if (!toolCalls.byTool) return []
    return Object.entries(toolCalls.byTool)
      .sort(([, a], [, b]) => b - a)
      .slice(0, 5) // Top 5 tools
  }, [toolCalls.byTool])

  const isCompact = variant === 'compact'

  return (
    <div className="usage-summary" style={styles.container}>
      {/* Header with period */}
      <div className="usage-summary__header" style={styles.header}>
        <h3 style={styles.title}>Usage Summary</h3>
        {showPeriod && (
          <span className="usage-summary__period" style={styles.period}>
            {formatPeriod(periodStart, periodEnd)}
          </span>
        )}
      </div>

      {/* Main stats grid */}
      <div className="usage-summary__grid" style={{ ...styles.grid, ...(isCompact ? styles.gridCompact : {}) }}>
        {/* Cost card */}
        <div className="usage-summary__card" style={styles.card}>
          <DollarIcon />
          <div className="usage-summary__card-content" style={styles.cardContent}>
            <span className="usage-summary__card-value" style={styles.cardValue}>
              {formatCost(usage.totalCostUsd)}
            </span>
            <span className="usage-summary__card-label" style={styles.cardLabel}>
              Total Cost
            </span>
          </div>
        </div>

        {/* Tokens card */}
        <div className="usage-summary__card" style={styles.card}>
          <TokenIcon />
          <div className="usage-summary__card-content" style={styles.cardContent}>
            <span className="usage-summary__card-value" style={styles.cardValue}>
              {formatTokens(stats.totalTokens)}
            </span>
            <span className="usage-summary__card-label" style={styles.cardLabel}>
              Total Tokens
            </span>
          </div>
        </div>

        {/* Requests card */}
        <div className="usage-summary__card" style={styles.card}>
          <RequestIcon />
          <div className="usage-summary__card-content" style={styles.cardContent}>
            <span className="usage-summary__card-value" style={styles.cardValue}>
              {usage.totalRequests.toLocaleString()}
            </span>
            <span className="usage-summary__card-label" style={styles.cardLabel}>
              API Requests
            </span>
          </div>
        </div>

        {/* Tools card */}
        {showToolStats && (
          <div className="usage-summary__card" style={styles.card}>
            <ToolIcon />
            <div className="usage-summary__card-content" style={styles.cardContent}>
              <span className="usage-summary__card-value" style={styles.cardValue}>
                {toolCalls.totalCompleted.toLocaleString()}
              </span>
              <span className="usage-summary__card-label" style={styles.cardLabel}>
                Tool Calls
              </span>
            </div>
          </div>
        )}
      </div>

      {/* Detailed breakdown (not shown in compact mode) */}
      {!isCompact && (
        <>
          {/* Token breakdown */}
          <div className="usage-summary__section" style={styles.section}>
            <h4 className="usage-summary__section-title" style={styles.sectionTitle}>
              Token Breakdown
            </h4>
            <div className="usage-summary__breakdown" style={styles.breakdown}>
              <div className="usage-summary__breakdown-item" style={styles.breakdownItem}>
                <span className="usage-summary__breakdown-label" style={styles.breakdownLabel}>
                  Input Tokens
                </span>
                <span className="usage-summary__breakdown-value" style={styles.breakdownValue}>
                  {formatTokens(usage.totalTokensInput)}
                </span>
                <div className="usage-summary__breakdown-bar" style={styles.breakdownBar}>
                  <div
                    style={{
                      ...styles.breakdownFill,
                      width: `${(usage.totalTokensInput / stats.totalTokens) * 100 || 0}%`,
                      backgroundColor: '#79c0ff',
                    }}
                  />
                </div>
              </div>
              <div className="usage-summary__breakdown-item" style={styles.breakdownItem}>
                <span className="usage-summary__breakdown-label" style={styles.breakdownLabel}>
                  Output Tokens
                </span>
                <span className="usage-summary__breakdown-value" style={styles.breakdownValue}>
                  {formatTokens(usage.totalTokensOutput)}
                </span>
                <div className="usage-summary__breakdown-bar" style={styles.breakdownBar}>
                  <div
                    style={{
                      ...styles.breakdownFill,
                      width: `${(usage.totalTokensOutput / stats.totalTokens) * 100 || 0}%`,
                      backgroundColor: '#7ee787',
                    }}
                  />
                </div>
              </div>
            </div>
          </div>

          {/* Request stats */}
          <div className="usage-summary__section" style={styles.section}>
            <h4 className="usage-summary__section-title" style={styles.sectionTitle}>
              Request Statistics
            </h4>
            <div className="usage-summary__stats-row" style={styles.statsRow}>
              <div className="usage-summary__stat" style={styles.stat}>
                <span className="usage-summary__stat-value" style={{ ...styles.statValue, color: '#3fb950' }}>
                  {usage.successfulRequests.toLocaleString()}
                </span>
                <span className="usage-summary__stat-label" style={styles.statLabel}>Successful</span>
              </div>
              <div className="usage-summary__stat" style={styles.stat}>
                <span className="usage-summary__stat-value" style={{ ...styles.statValue, color: '#f85149' }}>
                  {usage.failedRequests.toLocaleString()}
                </span>
                <span className="usage-summary__stat-label" style={styles.statLabel}>Failed</span>
              </div>
              <div className="usage-summary__stat" style={styles.stat}>
                <span className="usage-summary__stat-value" style={styles.statValue}>
                  {stats.successRate.toFixed(1)}%
                </span>
                <span className="usage-summary__stat-label" style={styles.statLabel}>Success Rate</span>
              </div>
              <div className="usage-summary__stat" style={styles.stat}>
                <span className="usage-summary__stat-value" style={styles.statValue}>
                  {formatDuration(usage.averageDurationMs)}
                </span>
                <span className="usage-summary__stat-label" style={styles.statLabel}>Avg Duration</span>
              </div>
            </div>
          </div>

          {/* Provider breakdown */}
          {showProviderBreakdown && sortedProviders.length > 0 && (
            <div className="usage-summary__section" style={styles.section}>
              <h4 className="usage-summary__section-title" style={styles.sectionTitle}>
                By Provider
              </h4>
              <div className="usage-summary__providers" style={styles.providers}>
                {sortedProviders.map((provider) => (
                  <ProviderRow key={provider.provider} provider={provider} totalCost={usage.totalCostUsd} />
                ))}
              </div>
            </div>
          )}

          {/* Tool call stats */}
          {showToolStats && toolCalls.totalRequested > 0 && (
            <div className="usage-summary__section" style={styles.section}>
              <h4 className="usage-summary__section-title" style={styles.sectionTitle}>
                Tool Calls
              </h4>
              <div className="usage-summary__stats-row" style={styles.statsRow}>
                <div className="usage-summary__stat" style={styles.stat}>
                  <span className="usage-summary__stat-value" style={styles.statValue}>
                    {toolCalls.totalRequested.toLocaleString()}
                  </span>
                  <span className="usage-summary__stat-label" style={styles.statLabel}>Requested</span>
                </div>
                <div className="usage-summary__stat" style={styles.stat}>
                  <span className="usage-summary__stat-value" style={{ ...styles.statValue, color: '#3fb950' }}>
                    {toolCalls.totalApproved.toLocaleString()}
                  </span>
                  <span className="usage-summary__stat-label" style={styles.statLabel}>Approved</span>
                </div>
                <div className="usage-summary__stat" style={styles.stat}>
                  <span className="usage-summary__stat-value" style={{ ...styles.statValue, color: '#a371f7' }}>
                    {toolCalls.totalAutoApproved.toLocaleString()}
                  </span>
                  <span className="usage-summary__stat-label" style={styles.statLabel}>Auto-approved</span>
                </div>
                <div className="usage-summary__stat" style={styles.stat}>
                  <span className="usage-summary__stat-value" style={{ ...styles.statValue, color: '#f85149' }}>
                    {toolCalls.totalRejected.toLocaleString()}
                  </span>
                  <span className="usage-summary__stat-label" style={styles.statLabel}>Rejected</span>
                </div>
              </div>

              {/* Top tools */}
              {sortedTools.length > 0 && (
                <div className="usage-summary__top-tools" style={styles.topTools}>
                  <span className="usage-summary__top-tools-label" style={styles.topToolsLabel}>
                    Top Tools:
                  </span>
                  <div className="usage-summary__tool-tags" style={styles.toolTags}>
                    {sortedTools.map(([tool, count]) => (
                      <span key={tool} className="usage-summary__tool-tag" style={styles.toolTag}>
                        {tool} <span style={styles.toolCount}>({count})</span>
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Session counts */}
          {(activeSessionCount !== undefined || totalSessionCount !== undefined) && (
            <div className="usage-summary__footer" style={styles.footer}>
              {activeSessionCount !== undefined && (
                <span className="usage-summary__footer-item" style={styles.footerItem}>
                  <span style={{ color: '#3fb950' }}>{activeSessionCount}</span> active sessions
                </span>
              )}
              {totalSessionCount !== undefined && (
                <span className="usage-summary__footer-item" style={styles.footerItem}>
                  <span style={{ color: '#c9d1d9' }}>{totalSessionCount}</span> total sessions
                </span>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}

/**
 * Provider row component for breakdown display
 */
interface ProviderRowProps {
  provider: ProviderStats
  totalCost: number
}

const ProviderRow: React.FC<ProviderRowProps> = ({ provider, totalCost }) => {
  const costPercent = totalCost > 0 ? (provider.totalCostUsd / totalCost) * 100 : 0

  return (
    <div className="usage-summary__provider-row" style={styles.providerRow}>
      <div className="usage-summary__provider-info" style={styles.providerInfo}>
        <span className="usage-summary__provider-name" style={styles.providerName}>
          {provider.provider}
        </span>
        <span className="usage-summary__provider-requests" style={styles.providerRequests}>
          {provider.totalRequests.toLocaleString()} requests
        </span>
      </div>
      <div className="usage-summary__provider-bar-container" style={styles.providerBarContainer}>
        <div className="usage-summary__provider-bar" style={styles.providerBar}>
          <div
            style={{
              ...styles.providerBarFill,
              width: `${costPercent}%`,
            }}
          />
        </div>
      </div>
      <div className="usage-summary__provider-cost" style={styles.providerCost}>
        <span className="usage-summary__provider-cost-value" style={styles.providerCostValue}>
          {formatCost(provider.totalCostUsd)}
        </span>
        <span className="usage-summary__provider-cost-percent" style={styles.providerCostPercent}>
          {costPercent.toFixed(1)}%
        </span>
      </div>
    </div>
  )
}

// Icon components
const DollarIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#3fb950" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <line x1="12" y1="1" x2="12" y2="23" />
    <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
  </svg>
)

const TokenIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#79c0ff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="12" r="10" />
    <path d="M8 12h8" />
    <path d="M12 8v8" />
  </svg>
)

const RequestIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#a371f7" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M22 2L11 13" />
    <path d="M22 2L15 22L11 13L2 9L22 2Z" />
  </svg>
)

const ToolIcon: React.FC = () => (
  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#ffd33d" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
    backgroundColor: '#161b22',
    borderRadius: '8px',
    padding: '16px',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: '4px',
  },
  title: {
    margin: 0,
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  period: {
    fontSize: '12px',
    color: '#8b949e',
    backgroundColor: '#21262d',
    padding: '4px 8px',
    borderRadius: '4px',
  },
  grid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(4, 1fr)',
    gap: '12px',
  },
  gridCompact: {
    gridTemplateColumns: 'repeat(2, 1fr)',
  },
  card: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    backgroundColor: '#21262d',
    borderRadius: '8px',
    padding: '12px',
  },
  cardContent: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  cardValue: {
    fontSize: '18px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  cardLabel: {
    fontSize: '11px',
    color: '#8b949e',
  },
  section: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    paddingTop: '12px',
    borderTop: '1px solid #30363d',
  },
  sectionTitle: {
    margin: 0,
    fontSize: '12px',
    fontWeight: 600,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  breakdown: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  breakdownItem: {
    display: 'grid',
    gridTemplateColumns: '100px 80px 1fr',
    alignItems: 'center',
    gap: '12px',
  },
  breakdownLabel: {
    fontSize: '12px',
    color: '#8b949e',
  },
  breakdownValue: {
    fontSize: '12px',
    fontWeight: 500,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    textAlign: 'right',
  },
  breakdownBar: {
    height: '6px',
    backgroundColor: '#30363d',
    borderRadius: '3px',
    overflow: 'hidden',
  },
  breakdownFill: {
    height: '100%',
    borderRadius: '3px',
    transition: 'width 0.3s ease',
  },
  statsRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(4, 1fr)',
    gap: '16px',
  },
  stat: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: '4px',
    padding: '8px',
    backgroundColor: '#21262d',
    borderRadius: '6px',
  },
  statValue: {
    fontSize: '16px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  statLabel: {
    fontSize: '10px',
    color: '#8b949e',
    textAlign: 'center',
  },
  providers: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
  },
  providerRow: {
    display: 'grid',
    gridTemplateColumns: '120px 1fr 100px',
    alignItems: 'center',
    gap: '12px',
    padding: '8px 12px',
    backgroundColor: '#21262d',
    borderRadius: '6px',
  },
  providerInfo: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  providerName: {
    fontSize: '12px',
    fontWeight: 500,
    color: '#c9d1d9',
    textTransform: 'capitalize',
  },
  providerRequests: {
    fontSize: '10px',
    color: '#6e7681',
  },
  providerBarContainer: {
    width: '100%',
  },
  providerBar: {
    height: '8px',
    backgroundColor: '#30363d',
    borderRadius: '4px',
    overflow: 'hidden',
  },
  providerBarFill: {
    height: '100%',
    backgroundColor: '#238636',
    borderRadius: '4px',
    transition: 'width 0.3s ease',
  },
  providerCost: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: '2px',
  },
  providerCostValue: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  providerCostPercent: {
    fontSize: '10px',
    color: '#6e7681',
  },
  topTools: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    marginTop: '8px',
    paddingTop: '8px',
    borderTop: '1px solid #30363d',
  },
  topToolsLabel: {
    fontSize: '10px',
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  toolTags: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: '6px',
  },
  toolTag: {
    fontSize: '11px',
    color: '#c9d1d9',
    backgroundColor: '#30363d',
    padding: '4px 8px',
    borderRadius: '4px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  toolCount: {
    color: '#8b949e',
    fontWeight: 400,
  },
  footer: {
    display: 'flex',
    justifyContent: 'center',
    gap: '16px',
    paddingTop: '12px',
    borderTop: '1px solid #30363d',
  },
  footerItem: {
    fontSize: '11px',
    color: '#8b949e',
  },
}

export default UsageSummary
