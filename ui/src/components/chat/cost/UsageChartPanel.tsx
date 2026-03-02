/**
 * UsageChartPanel Component
 *
 * A comprehensive dashboard panel for displaying usage charts.
 * Combines provider breakdown, token history, and cost breakdown views.
 *
 * @module components/chat/cost/UsageChartPanel
 */

import React, { useState, useEffect, useCallback, useMemo } from 'react'
import type { ProviderStats, ToolCallStats, UsageStats } from '../../../types/chat'
import UsageBarChart, { formatCost, formatNumber } from './UsageBarChart'
import ProviderUsageChart from './ProviderUsageChart'
import TokenHistoryChart, { type TokenHistoryDataPoint } from './TokenHistoryChart'

export interface UsageChartPanelProps {
  /** Usage statistics data */
  usage?: UsageStats
  /** Provider statistics for breakdown chart */
  providers?: ProviderStats[]
  /** Tool call statistics */
  toolCalls?: ToolCallStats
  /** Token history data points */
  tokenHistory?: TokenHistoryDataPoint[]
  /** Session ID for context */
  sessionId?: string
  /** API base URL for fetching data */
  apiBaseUrl?: string
  /** Whether to auto-refresh data */
  autoRefresh?: boolean
  /** Auto-refresh interval in milliseconds */
  refreshInterval?: number
  /** Whether the panel is loading */
  loading?: boolean
  /** Error message to display */
  error?: string
  /** Callback when refresh is requested */
  onRefresh?: () => void
}

type ChartView = 'overview' | 'providers' | 'tokens' | 'tools'

const UsageChartPanel: React.FC<UsageChartPanelProps> = ({
  usage,
  providers = [],
  toolCalls,
  tokenHistory = [],
  sessionId,
  apiBaseUrl = '/api',
  autoRefresh = false,
  refreshInterval = 30000,
  loading = false,
  error,
  onRefresh,
}) => {
  const [activeView, setActiveView] = useState<ChartView>('overview')
  const [isCollapsed, setIsCollapsed] = useState(false)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)

  // Update last updated timestamp when data changes
  useEffect(() => {
    if (usage || providers.length > 0 || tokenHistory.length > 0) {
      setLastUpdated(new Date())
    }
  }, [usage, providers, tokenHistory])

  // Auto-refresh timer
  useEffect(() => {
    if (!autoRefresh || !onRefresh) return

    const timer = setInterval(() => {
      onRefresh()
    }, refreshInterval)

    return () => clearInterval(timer)
  }, [autoRefresh, refreshInterval, onRefresh])

  // Prepare data for cost breakdown chart
  const costBreakdownData = useMemo(() => {
    if (!providers.length) return []
    return providers.map(p => ({
      label: p.provider,
      value: p.totalCostUsd,
      sublabel: `${formatNumber(p.totalTokensInput + p.totalTokensOutput)} tokens`,
    }))
  }, [providers])

  // Prepare data for tool calls chart
  const toolCallsData = useMemo(() => {
    if (!toolCalls?.byTool) return []
    return Object.entries(toolCalls.byTool).map(([tool, count]) => ({
      label: tool,
      value: count,
    }))
  }, [toolCalls])

  // Tab buttons
  const tabs: { id: ChartView; label: string }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'providers', label: 'Providers' },
    { id: 'tokens', label: 'Tokens' },
    { id: 'tools', label: 'Tools' },
  ]

  // Format last updated time
  const formatLastUpdated = (): string => {
    if (!lastUpdated) return 'Never'
    const now = new Date()
    const diff = Math.floor((now.getTime() - lastUpdated.getTime()) / 1000)
    if (diff < 60) return `${diff}s ago`
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
    return lastUpdated.toLocaleTimeString()
  }

  return (
    <div className="usage-chart-panel" style={styles.container}>
      {/* Header */}
      <div className="usage-chart-panel__header" style={styles.header}>
        <div style={styles.headerLeft}>
          <button
            onClick={() => setIsCollapsed(!isCollapsed)}
            style={styles.collapseButton}
            aria-label={isCollapsed ? 'Expand' : 'Collapse'}
          >
            <ChevronIcon isCollapsed={isCollapsed} />
          </button>
          <h3 style={styles.title}>Usage Analytics</h3>
          {loading && <span style={styles.loadingIndicator}>Loading...</span>}
        </div>

        <div style={styles.headerRight}>
          <span style={styles.lastUpdated}>
            Updated: {formatLastUpdated()}
          </span>
          {onRefresh && (
            <button
              onClick={onRefresh}
              disabled={loading}
              style={styles.refreshButton}
              title="Refresh data"
            >
              <RefreshIcon />
            </button>
          )}
        </div>
      </div>

      {/* Collapsed state */}
      {isCollapsed && (
        <div style={styles.collapsedSummary}>
          {usage && (
            <>
              <span style={styles.collapsedItem}>
                Cost: <strong>{formatCost(usage.totalCostUsd)}</strong>
              </span>
              <span style={styles.collapsedItem}>
                Tokens: <strong>{formatNumber(usage.totalTokensInput + usage.totalTokensOutput)}</strong>
              </span>
              <span style={styles.collapsedItem}>
                Requests: <strong>{usage.totalRequests}</strong>
              </span>
            </>
          )}
        </div>
      )}

      {/* Expanded content */}
      {!isCollapsed && (
        <>
          {/* Error state */}
          {error && (
            <div style={styles.error}>
              <ErrorIcon />
              <span>{error}</span>
            </div>
          )}

          {/* Tab navigation */}
          <div className="usage-chart-panel__tabs" style={styles.tabs}>
            {tabs.map(tab => (
              <button
                key={tab.id}
                onClick={() => setActiveView(tab.id)}
                style={{
                  ...styles.tab,
                  ...(activeView === tab.id ? styles.tabActive : {}),
                }}
              >
                {tab.label}
              </button>
            ))}
          </div>

          {/* Chart content */}
          <div className="usage-chart-panel__content" style={styles.content}>
            {activeView === 'overview' && (
              <OverviewSection
                usage={usage}
                providers={providers}
                toolCalls={toolCalls}
              />
            )}

            {activeView === 'providers' && (
              <ProviderUsageChart
                providers={providers}
                totalCost={usage?.totalCostUsd}
                showDetails={true}
              />
            )}

            {activeView === 'tokens' && (
              <div style={styles.tokensSection}>
                <TokenHistoryChart
                  data={tokenHistory}
                  height={180}
                  showCost={true}
                  showFill={true}
                />
              </div>
            )}

            {activeView === 'tools' && (
              <div style={styles.toolsSection}>
                {toolCalls && (
                  <>
                    <ToolCallSummary stats={toolCalls} />
                    <UsageBarChart
                      data={toolCallsData}
                      title="Calls by Tool"
                      formatValue={v => v.toString()}
                      showPercentage={true}
                      emptyMessage="No tool calls recorded"
                    />
                  </>
                )}
                {!toolCalls && (
                  <div style={styles.empty}>No tool call data available</div>
                )}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

// Overview Section Component
interface OverviewSectionProps {
  usage?: UsageStats
  providers?: ProviderStats[]
  toolCalls?: ToolCallStats
}

const OverviewSection: React.FC<OverviewSectionProps> = ({
  usage,
  providers = [],
  toolCalls,
}) => {
  if (!usage) {
    return <div style={styles.empty}>No usage data available</div>
  }

  return (
    <div style={styles.overview}>
      {/* Key metrics row */}
      <div style={styles.metricsRow}>
        <MetricCard
          label="Total Cost"
          value={formatCost(usage.totalCostUsd)}
          color="#ffd33d"
        />
        <MetricCard
          label="Total Tokens"
          value={formatNumber(usage.totalTokensInput + usage.totalTokensOutput)}
          sublabel={`${formatNumber(usage.totalTokensInput)} in / ${formatNumber(usage.totalTokensOutput)} out`}
          color="#79c0ff"
        />
        <MetricCard
          label="Requests"
          value={usage.totalRequests.toString()}
          sublabel={`${usage.successfulRequests} success / ${usage.failedRequests} failed`}
          color="#7ee787"
        />
        <MetricCard
          label="Avg Duration"
          value={`${usage.averageDurationMs.toFixed(0)}ms`}
          color="#d2a8ff"
        />
      </div>

      {/* Cost breakdown mini chart */}
      {providers.length > 0 && (
        <div style={styles.miniChart}>
          <ProviderUsageChart
            providers={providers}
            totalCost={usage.totalCostUsd}
            size={80}
            showDetails={false}
            title="Cost by Provider"
          />
        </div>
      )}
    </div>
  )
}

// Metric Card Component
interface MetricCardProps {
  label: string
  value: string
  sublabel?: string
  color?: string
}

const MetricCard: React.FC<MetricCardProps> = ({
  label,
  value,
  sublabel,
  color = '#c9d1d9',
}) => (
  <div style={styles.metricCard}>
    <span style={styles.metricLabel}>{label}</span>
    <span style={{ ...styles.metricValue, color }}>{value}</span>
    {sublabel && <span style={styles.metricSublabel}>{sublabel}</span>}
  </div>
)

// Tool Call Summary Component
interface ToolCallSummaryProps {
  stats: ToolCallStats
}

const ToolCallSummary: React.FC<ToolCallSummaryProps> = ({ stats }) => (
  <div style={styles.toolSummary}>
    <div style={styles.toolSummaryRow}>
      <span style={styles.toolSummaryLabel}>Total Requested</span>
      <span style={styles.toolSummaryValue}>{stats.totalRequested}</span>
    </div>
    <div style={styles.toolSummaryRow}>
      <span style={{ ...styles.toolSummaryLabel, color: '#3fb950' }}>Approved</span>
      <span style={styles.toolSummaryValue}>{stats.totalApproved}</span>
    </div>
    <div style={styles.toolSummaryRow}>
      <span style={{ ...styles.toolSummaryLabel, color: '#f85149' }}>Rejected</span>
      <span style={styles.toolSummaryValue}>{stats.totalRejected}</span>
    </div>
    <div style={styles.toolSummaryRow}>
      <span style={{ ...styles.toolSummaryLabel, color: '#79c0ff' }}>Auto-Approved</span>
      <span style={styles.toolSummaryValue}>{stats.totalAutoApproved}</span>
    </div>
    <div style={styles.toolSummaryRow}>
      <span style={{ ...styles.toolSummaryLabel, color: '#7ee787' }}>Completed</span>
      <span style={styles.toolSummaryValue}>{stats.totalCompleted}</span>
    </div>
    <div style={styles.toolSummaryRow}>
      <span style={{ ...styles.toolSummaryLabel, color: '#f85149' }}>Failed</span>
      <span style={styles.toolSummaryValue}>{stats.totalFailed}</span>
    </div>
  </div>
)

// Icon Components
const ChevronIcon: React.FC<{ isCollapsed: boolean }> = ({ isCollapsed }) => (
  <svg
    width="12"
    height="12"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{
      transform: isCollapsed ? 'rotate(-90deg)' : 'rotate(0deg)',
      transition: 'transform 0.2s ease',
    }}
  >
    <polyline points="6 9 12 15 18 9" />
  </svg>
)

const RefreshIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M23 4v6h-6M1 20v-6h6" />
    <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#f85149" strokeWidth="2">
    <circle cx="12" cy="12" r="10" />
    <line x1="12" y1="8" x2="12" y2="12" />
    <line x1="12" y1="16" x2="12.01" y2="16" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '6px',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '10px 12px',
    backgroundColor: '#21262d',
    borderBottom: '1px solid #30363d',
  },
  headerLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  headerRight: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  collapseButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '20px',
    height: '20px',
    padding: 0,
    backgroundColor: 'transparent',
    border: 'none',
    color: '#8b949e',
    cursor: 'pointer',
    borderRadius: '4px',
  },
  title: {
    margin: 0,
    fontSize: '13px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  loadingIndicator: {
    fontSize: '10px',
    color: '#8b949e',
    fontStyle: 'italic',
  },
  lastUpdated: {
    fontSize: '10px',
    color: '#6e7681',
  },
  refreshButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '24px',
    height: '24px',
    padding: 0,
    backgroundColor: 'transparent',
    border: '1px solid #30363d',
    borderRadius: '4px',
    color: '#8b949e',
    cursor: 'pointer',
  },
  collapsedSummary: {
    display: 'flex',
    gap: '16px',
    padding: '8px 12px',
    fontSize: '11px',
    color: '#8b949e',
  },
  collapsedItem: {
    display: 'flex',
    gap: '4px',
  },
  tabs: {
    display: 'flex',
    gap: '2px',
    padding: '8px 12px',
    borderBottom: '1px solid #21262d',
  },
  tab: {
    padding: '6px 12px',
    fontSize: '11px',
    fontWeight: 500,
    color: '#8b949e',
    backgroundColor: 'transparent',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  tabActive: {
    color: '#c9d1d9',
    backgroundColor: '#30363d',
  },
  content: {
    padding: '12px',
  },
  error: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 12px',
    backgroundColor: 'rgba(248, 81, 73, 0.1)',
    borderBottom: '1px solid #f85149',
    fontSize: '11px',
    color: '#f85149',
  },
  empty: {
    fontSize: '11px',
    color: '#6e7681',
    fontStyle: 'italic',
    padding: '24px 0',
    textAlign: 'center',
  },
  overview: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  metricsRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(2, 1fr)',
    gap: '12px',
  },
  metricCard: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    padding: '10px',
    backgroundColor: '#21262d',
    borderRadius: '6px',
  },
  metricLabel: {
    fontSize: '9px',
    fontWeight: 500,
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  metricValue: {
    fontSize: '16px',
    fontWeight: 600,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  metricSublabel: {
    fontSize: '9px',
    color: '#6e7681',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  miniChart: {
    paddingTop: '8px',
    borderTop: '1px solid #21262d',
  },
  tokensSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  toolsSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: '16px',
  },
  toolSummary: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, 1fr)',
    gap: '8px',
    padding: '12px',
    backgroundColor: '#21262d',
    borderRadius: '6px',
  },
  toolSummaryRow: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: '2px',
  },
  toolSummaryLabel: {
    fontSize: '9px',
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  toolSummaryValue: {
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
}

export default UsageChartPanel
