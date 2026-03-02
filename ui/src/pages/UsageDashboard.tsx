/**
 * UsageDashboard Component
 *
 * Comprehensive dashboard for displaying cost and token usage statistics.
 * Combines all usage charts with time range filtering and real-time updates.
 *
 * @module pages/UsageDashboard
 */

import React, { useState, useCallback, useMemo } from 'react'
import {
  UsageSummary,
  CostTimelineChart,
  TokenHistoryChart,
  ProviderUsageChart,
  UsageBarChart,
  formatCost,
  formatNumber,
} from '../components/chat/cost'
import TimeRangeSelector, {
  type TimeRange,
  type TimeRangePreset,
} from '../components/chat/cost/TimeRangeSelector'
import { useUsageData } from '../hooks/useUsageData'
import type { UsageSummaryData } from '../types/chat'

/**
 * Dashboard configuration
 */
export interface UsageDashboardConfig {
  /** API base URL */
  apiBaseUrl?: string
  /** Auto-refresh interval in milliseconds (0 to disable) */
  refreshInterval?: number
  /** Default time range preset */
  defaultTimeRange?: TimeRangePreset
  /** Session ID for session-specific view */
  sessionId?: string
  /** Budget limit for cost charts */
  budgetLimit?: number
}

/**
 * Props for UsageDashboard
 */
export interface UsageDashboardProps {
  /** Dashboard configuration */
  config?: UsageDashboardConfig
}

/**
 * Dashboard section type for navigation
 */
type DashboardSection = 'overview' | 'cost' | 'tokens' | 'providers' | 'tools'

/**
 * Default time range (last 7 days)
 */
const DEFAULT_TIME_RANGE: TimeRange = {
  since: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
  until: new Date(),
  preset: 'last_7d',
}

/**
 * UsageDashboard Component
 *
 * Provides a comprehensive view of usage statistics including:
 * - Cost summary and timeline
 * - Token usage breakdown
 * - Provider comparison
 * - Tool call analytics
 */
const UsageDashboard: React.FC<UsageDashboardProps> = ({ config = {} }) => {
  const {
    apiBaseUrl = '/api',
    refreshInterval = 30000,
    defaultTimeRange = 'last_7d',
    sessionId,
    budgetLimit,
  } = config

  // State
  const [timeRange, setTimeRange] = useState<TimeRange>({
    ...DEFAULT_TIME_RANGE,
    preset: defaultTimeRange,
  })
  const [activeSection, setActiveSection] = useState<DashboardSection>('overview')
  const [isAutoRefresh, setIsAutoRefresh] = useState(true)

  // Fetch usage data with current filters
  const usageData = useUsageData({
    apiBaseUrl,
    sessionId,
    autoFetch: true,
    refreshInterval: isAutoRefresh ? refreshInterval : 0,
    since: timeRange.since ?? undefined,
    until: timeRange.until ?? undefined,
  })

  // Handle time range change
  const handleTimeRangeChange = useCallback((range: TimeRange) => {
    setTimeRange(range)
  }, [])

  // Handle manual refresh
  const handleRefresh = useCallback(() => {
    usageData.fetchAll()
  }, [usageData])

  // Toggle auto-refresh
  const handleToggleAutoRefresh = useCallback(() => {
    setIsAutoRefresh(prev => !prev)
  }, [])

  // Build usage summary data for UsageSummary component
  const usageSummaryData: UsageSummaryData | null = useMemo(() => {
    if (!usageData.usage) return null

    return {
      usage: usageData.usage,
      toolCalls: usageData.toolCalls ?? {
        totalRequested: 0,
        totalApproved: 0,
        totalRejected: 0,
        totalAutoApproved: 0,
        totalCompleted: 0,
        totalFailed: 0,
      },
      periodStart: timeRange.since?.toISOString(),
      periodEnd: timeRange.until?.toISOString(),
    }
  }, [usageData.usage, usageData.toolCalls, timeRange])

  // Build provider cost breakdown for bar chart
  const providerCostData = useMemo(() => {
    if (!usageData.providers.length) return []
    return usageData.providers.map(p => ({
      label: p.provider,
      value: p.totalCostUsd,
      sublabel: `${formatNumber(p.totalTokensInput + p.totalTokensOutput)} tokens`,
    }))
  }, [usageData.providers])

  // Build tool usage data for bar chart
  const toolUsageData = useMemo(() => {
    if (!usageData.toolCalls?.byTool) return []
    return Object.entries(usageData.toolCalls.byTool)
      .sort(([, a], [, b]) => b - a)
      .map(([tool, count]) => ({
        label: tool,
        value: count,
      }))
  }, [usageData.toolCalls])

  // Navigation tabs
  const tabs: Array<{ id: DashboardSection; label: string }> = [
    { id: 'overview', label: 'Overview' },
    { id: 'cost', label: 'Cost' },
    { id: 'tokens', label: 'Tokens' },
    { id: 'providers', label: 'Providers' },
    { id: 'tools', label: 'Tools' },
  ]

  return (
    <div className="usage-dashboard" style={styles.container}>
      {/* Header */}
      <header className="usage-dashboard__header" style={styles.header}>
        <div className="usage-dashboard__title-section" style={styles.titleSection}>
          <h1 style={styles.title}>Usage Dashboard</h1>
          {sessionId && (
            <span style={styles.sessionBadge}>Session: {sessionId.slice(0, 8)}...</span>
          )}
        </div>

        <div className="usage-dashboard__controls" style={styles.controls}>
          {/* Time range selector */}
          <TimeRangeSelector
            value={timeRange}
            onChange={handleTimeRangeChange}
            disabled={usageData.loading}
            compact={true}
          />

          {/* Auto-refresh toggle */}
          <button
            onClick={handleToggleAutoRefresh}
            style={{
              ...styles.controlButton,
              ...(isAutoRefresh ? styles.controlButtonActive : {}),
            }}
            title={isAutoRefresh ? 'Auto-refresh enabled' : 'Auto-refresh disabled'}
          >
            <RefreshIcon />
            <span style={styles.controlButtonText}>
              {isAutoRefresh ? 'Auto' : 'Manual'}
            </span>
          </button>

          {/* Manual refresh button */}
          <button
            onClick={handleRefresh}
            disabled={usageData.loading}
            style={{
              ...styles.controlButton,
              ...(usageData.loading ? styles.controlButtonDisabled : {}),
            }}
            title="Refresh now"
          >
            {usageData.loading ? <LoadingSpinner /> : <RefreshIcon />}
          </button>
        </div>
      </header>

      {/* Last updated indicator */}
      <div className="usage-dashboard__status" style={styles.status}>
        {usageData.error ? (
          <span style={styles.errorText}>
            <ErrorIcon /> {usageData.error}
          </span>
        ) : usageData.lastUpdated ? (
          <span style={styles.lastUpdated}>
            Last updated: {usageData.lastUpdated.toLocaleTimeString()}
          </span>
        ) : null}
      </div>

      {/* Navigation tabs */}
      <nav className="usage-dashboard__nav" style={styles.nav}>
        {tabs.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveSection(tab.id)}
            style={{
              ...styles.navTab,
              ...(activeSection === tab.id ? styles.navTabActive : {}),
            }}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {/* Main content */}
      <main className="usage-dashboard__content" style={styles.content}>
        {/* Overview Section */}
        {activeSection === 'overview' && (
          <div className="usage-dashboard__overview" style={styles.section}>
            {/* Key metrics summary */}
            {usageSummaryData && (
              <UsageSummary
                data={usageSummaryData}
                variant="detailed"
                showProviderBreakdown={true}
                showToolStats={true}
                showPeriod={true}
              />
            )}

            {/* Quick charts row */}
            <div className="usage-dashboard__charts-row" style={styles.chartsRow}>
              <div className="usage-dashboard__chart-card" style={styles.chartCard}>
                <h3 style={styles.chartTitle}>Cost Timeline</h3>
                <CostTimelineChart
                  data={usageData.costTimeline}
                  height={200}
                  budgetLimit={budgetLimit}
                  showIterationBars={true}
                  showFill={true}
                  emptyMessage="No cost data for selected period"
                />
              </div>

              <div className="usage-dashboard__chart-card" style={styles.chartCard}>
                <h3 style={styles.chartTitle}>Provider Breakdown</h3>
                <ProviderUsageChart
                  providers={usageData.providers}
                  totalCost={usageData.usage?.totalCostUsd}
                  showDetails={true}
                />
              </div>
            </div>
          </div>
        )}

        {/* Cost Section */}
        {activeSection === 'cost' && (
          <div className="usage-dashboard__cost" style={styles.section}>
            <div className="usage-dashboard__chart-card" style={styles.chartCardFull}>
              <h3 style={styles.chartTitle}>Cost Timeline</h3>
              <CostTimelineChart
                data={usageData.costTimeline}
                height={300}
                budgetLimit={budgetLimit}
                showIterationBars={true}
                showFill={true}
                maxPoints={50}
                emptyMessage="No cost data for selected period"
              />
            </div>

            <div className="usage-dashboard__charts-row" style={styles.chartsRow}>
              <div className="usage-dashboard__chart-card" style={styles.chartCard}>
                <h3 style={styles.chartTitle}>Cost by Provider</h3>
                <UsageBarChart
                  data={providerCostData}
                  formatValue={v => formatCost(v)}
                  showPercentage={true}
                  emptyMessage="No provider cost data"
                />
              </div>

              <div className="usage-dashboard__chart-card" style={styles.chartCard}>
                <h3 style={styles.chartTitle}>Cost Distribution</h3>
                <ProviderUsageChart
                  providers={usageData.providers}
                  totalCost={usageData.usage?.totalCostUsd}
                  size={150}
                  showDetails={true}
                />
              </div>
            </div>
          </div>
        )}

        {/* Tokens Section */}
        {activeSection === 'tokens' && (
          <div className="usage-dashboard__tokens" style={styles.section}>
            <div className="usage-dashboard__chart-card" style={styles.chartCardFull}>
              <h3 style={styles.chartTitle}>Token History</h3>
              <TokenHistoryChart
                data={usageData.tokenHistory}
                height={300}
                showCost={true}
                showFill={true}
              />
            </div>

            {/* Token breakdown cards */}
            <div className="usage-dashboard__metrics-row" style={styles.metricsRow}>
              <MetricCard
                label="Total Tokens"
                value={formatNumber(
                  (usageData.usage?.totalTokensInput ?? 0) +
                    (usageData.usage?.totalTokensOutput ?? 0)
                )}
                color="#79c0ff"
              />
              <MetricCard
                label="Input Tokens"
                value={formatNumber(usageData.usage?.totalTokensInput ?? 0)}
                color="#79c0ff"
                sublabel={`${(
                  ((usageData.usage?.totalTokensInput ?? 0) /
                    ((usageData.usage?.totalTokensInput ?? 0) +
                      (usageData.usage?.totalTokensOutput ?? 1))) *
                  100
                ).toFixed(1)}% of total`}
              />
              <MetricCard
                label="Output Tokens"
                value={formatNumber(usageData.usage?.totalTokensOutput ?? 0)}
                color="#7ee787"
                sublabel={`${(
                  ((usageData.usage?.totalTokensOutput ?? 0) /
                    ((usageData.usage?.totalTokensInput ?? 1) +
                      (usageData.usage?.totalTokensOutput ?? 0))) *
                  100
                ).toFixed(1)}% of total`}
              />
              <MetricCard
                label="Avg Per Request"
                value={formatNumber(
                  usageData.usage?.totalRequests
                    ? Math.round(
                        ((usageData.usage.totalTokensInput ?? 0) +
                          (usageData.usage.totalTokensOutput ?? 0)) /
                          usageData.usage.totalRequests
                      )
                    : 0
                )}
                color="#d2a8ff"
              />
            </div>
          </div>
        )}

        {/* Providers Section */}
        {activeSection === 'providers' && (
          <div className="usage-dashboard__providers" style={styles.section}>
            <div className="usage-dashboard__charts-row" style={styles.chartsRow}>
              <div className="usage-dashboard__chart-card" style={styles.chartCard}>
                <h3 style={styles.chartTitle}>Cost by Provider</h3>
                <ProviderUsageChart
                  providers={usageData.providers}
                  totalCost={usageData.usage?.totalCostUsd}
                  size={180}
                  showDetails={true}
                  title=""
                />
              </div>

              <div className="usage-dashboard__chart-card" style={styles.chartCard}>
                <h3 style={styles.chartTitle}>Tokens by Provider</h3>
                <UsageBarChart
                  data={usageData.providers.map(p => ({
                    label: p.provider,
                    value: p.totalTokensInput + p.totalTokensOutput,
                    sublabel: formatCost(p.totalCostUsd),
                  }))}
                  title=""
                  formatValue={v => formatNumber(v)}
                  showPercentage={true}
                  emptyMessage="No provider data"
                />
              </div>
            </div>

            {/* Provider detail table */}
            <div className="usage-dashboard__chart-card" style={styles.chartCardFull}>
              <h3 style={styles.chartTitle}>Provider Details</h3>
              <ProviderTable providers={usageData.providers} />
            </div>
          </div>
        )}

        {/* Tools Section */}
        {activeSection === 'tools' && (
          <div className="usage-dashboard__tools" style={styles.section}>
            {/* Tool call summary */}
            {usageData.toolCalls && (
              <div className="usage-dashboard__metrics-row" style={styles.metricsRow}>
                <MetricCard
                  label="Total Requested"
                  value={usageData.toolCalls.totalRequested.toString()}
                  color="#8b949e"
                />
                <MetricCard
                  label="Approved"
                  value={usageData.toolCalls.totalApproved.toString()}
                  color="#3fb950"
                />
                <MetricCard
                  label="Auto-Approved"
                  value={usageData.toolCalls.totalAutoApproved.toString()}
                  color="#79c0ff"
                />
                <MetricCard
                  label="Rejected"
                  value={usageData.toolCalls.totalRejected.toString()}
                  color="#f85149"
                />
                <MetricCard
                  label="Completed"
                  value={usageData.toolCalls.totalCompleted.toString()}
                  color="#7ee787"
                />
                <MetricCard
                  label="Failed"
                  value={usageData.toolCalls.totalFailed.toString()}
                  color="#f85149"
                />
              </div>
            )}

            {/* Tool usage breakdown */}
            <div className="usage-dashboard__chart-card" style={styles.chartCardFull}>
              <h3 style={styles.chartTitle}>Tool Usage Distribution</h3>
              <UsageBarChart
                data={toolUsageData}
                title=""
                formatValue={v => v.toString()}
                showPercentage={true}
                emptyMessage="No tool usage data"
              />
            </div>
          </div>
        )}
      </main>
    </div>
  )
}

/**
 * Metric Card Component
 */
interface MetricCardProps {
  label: string
  value: string
  color?: string
  sublabel?: string
}

const MetricCard: React.FC<MetricCardProps> = ({
  label,
  value,
  color = '#c9d1d9',
  sublabel,
}) => (
  <div className="metric-card" style={styles.metricCard}>
    <span style={styles.metricLabel}>{label}</span>
    <span style={{ ...styles.metricValue, color }}>{value}</span>
    {sublabel && <span style={styles.metricSublabel}>{sublabel}</span>}
  </div>
)

/**
 * Provider Table Component
 */
interface ProviderTableProps {
  providers: Array<{
    provider: string
    totalTokensInput: number
    totalTokensOutput: number
    totalCostUsd: number
    totalRequests: number
  }>
}

const ProviderTable: React.FC<ProviderTableProps> = ({ providers }) => {
  if (providers.length === 0) {
    return <div style={styles.emptyTable}>No provider data available</div>
  }

  return (
    <table style={styles.table}>
      <thead>
        <tr>
          <th style={styles.tableHeader}>Provider</th>
          <th style={{ ...styles.tableHeader, textAlign: 'right' }}>Input Tokens</th>
          <th style={{ ...styles.tableHeader, textAlign: 'right' }}>Output Tokens</th>
          <th style={{ ...styles.tableHeader, textAlign: 'right' }}>Total Tokens</th>
          <th style={{ ...styles.tableHeader, textAlign: 'right' }}>Cost</th>
          <th style={{ ...styles.tableHeader, textAlign: 'right' }}>Requests</th>
        </tr>
      </thead>
      <tbody>
        {providers.map(provider => (
          <tr key={provider.provider} style={styles.tableRow}>
            <td style={styles.tableCell}>{provider.provider}</td>
            <td style={{ ...styles.tableCell, textAlign: 'right' }}>
              {formatNumber(provider.totalTokensInput)}
            </td>
            <td style={{ ...styles.tableCell, textAlign: 'right' }}>
              {formatNumber(provider.totalTokensOutput)}
            </td>
            <td style={{ ...styles.tableCell, textAlign: 'right' }}>
              {formatNumber(provider.totalTokensInput + provider.totalTokensOutput)}
            </td>
            <td style={{ ...styles.tableCell, textAlign: 'right', color: '#ffd33d' }}>
              {formatCost(provider.totalCostUsd)}
            </td>
            <td style={{ ...styles.tableCell, textAlign: 'right' }}>
              {provider.totalRequests.toLocaleString()}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

/**
 * Icon Components
 */
const RefreshIcon: React.FC = () => (
  <svg
    width="14"
    height="14"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
  >
    <path d="M23 4v6h-6M1 20v-6h6" />
    <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
  </svg>
)

const ErrorIcon: React.FC = () => (
  <svg
    width="14"
    height="14"
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

const LoadingSpinner: React.FC = () => (
  <svg
    width="14"
    height="14"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{ animation: 'spin 1s linear infinite' }}
  >
    <path d="M21 12a9 9 0 1 1-6.219-8.56" />
  </svg>
)

/**
 * Styles
 */
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100vh',
    backgroundColor: '#0d1117',
    color: '#c9d1d9',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '16px 24px',
    borderBottom: '1px solid #21262d',
    backgroundColor: '#161b22',
  },
  titleSection: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  title: {
    margin: 0,
    fontSize: '20px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  sessionBadge: {
    fontSize: '11px',
    color: '#8b949e',
    backgroundColor: '#21262d',
    padding: '4px 8px',
    borderRadius: '12px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  controls: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  controlButton: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '6px 12px',
    fontSize: '12px',
    color: '#8b949e',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  controlButtonActive: {
    color: '#3fb950',
    borderColor: '#238636',
  },
  controlButtonDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
  controlButtonText: {
    fontSize: '11px',
    fontWeight: 500,
  },
  status: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-end',
    padding: '8px 24px',
    backgroundColor: '#161b22',
  },
  lastUpdated: {
    fontSize: '11px',
    color: '#6e7681',
  },
  errorText: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '11px',
    color: '#f85149',
  },
  nav: {
    display: 'flex',
    gap: '2px',
    padding: '0 24px',
    backgroundColor: '#161b22',
    borderBottom: '1px solid #21262d',
  },
  navTab: {
    padding: '12px 16px',
    fontSize: '13px',
    fontWeight: 500,
    color: '#8b949e',
    backgroundColor: 'transparent',
    border: 'none',
    borderBottom: '2px solid transparent',
    cursor: 'pointer',
    transition: 'all 0.2s ease',
  },
  navTabActive: {
    color: '#c9d1d9',
    borderBottomColor: '#f78166',
  },
  content: {
    flex: 1,
    padding: '24px',
    overflow: 'auto',
  },
  section: {
    display: 'flex',
    flexDirection: 'column',
    gap: '24px',
  },
  chartsRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))',
    gap: '24px',
  },
  chartCard: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    padding: '16px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '8px',
  },
  chartCardFull: {
    display: 'flex',
    flexDirection: 'column',
    gap: '12px',
    padding: '16px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '8px',
  },
  chartTitle: {
    margin: 0,
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  metricsRow: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(150px, 1fr))',
    gap: '12px',
  },
  metricCard: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
    padding: '16px',
    backgroundColor: '#21262d',
    borderRadius: '8px',
    textAlign: 'center',
  },
  metricLabel: {
    fontSize: '10px',
    fontWeight: 500,
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  metricValue: {
    fontSize: '24px',
    fontWeight: 600,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  metricSublabel: {
    fontSize: '10px',
    color: '#6e7681',
  },
  table: {
    width: '100%',
    borderCollapse: 'collapse',
    fontSize: '12px',
  },
  tableHeader: {
    padding: '10px 12px',
    fontWeight: 600,
    color: '#8b949e',
    textAlign: 'left',
    borderBottom: '1px solid #30363d',
    textTransform: 'uppercase',
    fontSize: '10px',
    letterSpacing: '0.5px',
  },
  tableRow: {
    borderBottom: '1px solid #21262d',
  },
  tableCell: {
    padding: '10px 12px',
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  emptyTable: {
    padding: '40px',
    textAlign: 'center',
    color: '#6e7681',
    fontStyle: 'italic',
  },
}

export default UsageDashboard
