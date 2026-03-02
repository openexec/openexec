/**
 * ProviderUsageChart Component
 *
 * Displays a donut chart visualization of usage breakdown by provider,
 * with accompanying legend showing cost and token details.
 *
 * @module components/chat/cost/ProviderUsageChart
 */

import React from 'react'
import type { ProviderStats } from '../../../types/chat'

export interface ProviderUsageChartProps {
  /** Provider statistics data */
  providers: ProviderStats[]
  /** Total cost in USD for all providers */
  totalCost?: number
  /** Size of the donut chart */
  size?: number
  /** Title for the chart */
  title?: string
  /** Show detailed breakdown in legend */
  showDetails?: boolean
  /** Empty state message */
  emptyMessage?: string
}

/** Color palette for providers */
const PROVIDER_COLORS: Record<string, string> = {
  openai: '#10a37f',     // OpenAI green
  anthropic: '#c96442', // Anthropic terracotta
  google: '#4285f4',    // Google blue
  gemini: '#8e44ad',    // Gemini purple
  azure: '#0078d4',     // Azure blue
  default: '#6e7681',   // Gray fallback
}

const getProviderColor = (provider: string, index: number): string => {
  const key = provider.toLowerCase()
  if (PROVIDER_COLORS[key]) return PROVIDER_COLORS[key]

  // Fallback colors for unknown providers
  const fallbackColors = ['#79c0ff', '#7ee787', '#d2a8ff', '#ff7b72', '#ffd33d']
  return fallbackColors[index % fallbackColors.length]
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

const ProviderUsageChart: React.FC<ProviderUsageChartProps> = ({
  providers,
  totalCost: providedTotalCost,
  size = 120,
  title = 'Usage by Provider',
  showDetails = true,
  emptyMessage = 'No usage data',
}) => {
  // Calculate total cost if not provided
  const totalCost = providedTotalCost ?? providers.reduce((sum, p) => sum + p.totalCostUsd, 0)

  // Calculate total tokens
  const totalTokens = providers.reduce(
    (sum, p) => sum + p.totalTokensInput + p.totalTokensOutput,
    0
  )

  // Calculate segment angles for donut chart
  const segments = providers.map((p, index) => {
    const percentage = totalCost > 0 ? (p.totalCostUsd / totalCost) * 100 : 0
    return {
      ...p,
      percentage,
      color: getProviderColor(p.provider, index),
    }
  })

  // SVG parameters
  const strokeWidth = 20
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const centerX = size / 2
  const centerY = size / 2

  // Calculate stroke dash offsets for each segment
  let accumulatedPercentage = 0
  const segmentsWithOffsets = segments.map(segment => {
    const dashLength = (segment.percentage / 100) * circumference
    const dashOffset = -(accumulatedPercentage / 100) * circumference
    accumulatedPercentage += segment.percentage
    return {
      ...segment,
      dashArray: `${dashLength} ${circumference}`,
      dashOffset,
    }
  })

  if (providers.length === 0 || totalCost === 0) {
    return (
      <div className="provider-usage-chart" style={styles.container}>
        {title && <h4 style={styles.title}>{title}</h4>}
        <div style={styles.empty}>{emptyMessage}</div>
      </div>
    )
  }

  return (
    <div className="provider-usage-chart" style={styles.container}>
      {title && <h4 style={styles.title}>{title}</h4>}

      <div className="provider-usage-chart__content" style={styles.content}>
        {/* Donut chart */}
        <div className="provider-usage-chart__chart" style={styles.chartWrapper}>
          <svg
            width={size}
            height={size}
            viewBox={`0 0 ${size} ${size}`}
            style={{ transform: 'rotate(-90deg)' }}
          >
            {/* Background circle */}
            <circle
              cx={centerX}
              cy={centerY}
              r={radius}
              fill="none"
              stroke="#21262d"
              strokeWidth={strokeWidth}
            />

            {/* Segment arcs */}
            {segmentsWithOffsets.map((segment, index) => (
              <circle
                key={segment.provider}
                cx={centerX}
                cy={centerY}
                r={radius}
                fill="none"
                stroke={segment.color}
                strokeWidth={strokeWidth}
                strokeDasharray={segment.dashArray}
                strokeDashoffset={segment.dashOffset}
                strokeLinecap="round"
                style={{ transition: 'stroke-dasharray 0.3s ease' }}
              />
            ))}
          </svg>

          {/* Center text */}
          <div className="provider-usage-chart__center" style={styles.center}>
            <span style={styles.centerValue}>{formatCost(totalCost)}</span>
            <span style={styles.centerLabel}>Total</span>
          </div>
        </div>

        {/* Legend */}
        <div className="provider-usage-chart__legend" style={styles.legend}>
          {segments.map((segment) => (
            <div
              key={segment.provider}
              className="provider-usage-chart__legend-item"
              style={styles.legendItem}
            >
              <div style={styles.legendHeader}>
                <span
                  className="provider-usage-chart__legend-color"
                  style={{
                    ...styles.legendColor,
                    backgroundColor: segment.color,
                  }}
                />
                <span className="provider-usage-chart__legend-label" style={styles.legendLabel}>
                  {segment.provider}
                </span>
                <span className="provider-usage-chart__legend-percent" style={styles.legendPercent}>
                  {segment.percentage.toFixed(1)}%
                </span>
              </div>

              {showDetails && (
                <div style={styles.legendDetails}>
                  <span style={styles.legendDetailItem}>
                    {formatCost(segment.totalCostUsd)}
                  </span>
                  <span style={styles.legendDetailItem}>
                    {formatTokens(segment.totalTokensInput + segment.totalTokensOutput)} tokens
                  </span>
                  <span style={styles.legendDetailItem}>
                    {segment.totalRequests} requests
                  </span>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Summary stats */}
      <div className="provider-usage-chart__summary" style={styles.summary}>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Total Tokens</span>
          <span style={styles.summaryValue}>{formatTokens(totalTokens)}</span>
        </div>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Providers</span>
          <span style={styles.summaryValue}>{providers.length}</span>
        </div>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Requests</span>
          <span style={styles.summaryValue}>
            {providers.reduce((sum, p) => sum + p.totalRequests, 0)}
          </span>
        </div>
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
  },
  title: {
    margin: 0,
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  content: {
    display: 'flex',
    gap: '16px',
    alignItems: 'flex-start',
  },
  chartWrapper: {
    position: 'relative',
    flexShrink: 0,
  },
  center: {
    position: 'absolute',
    top: '50%',
    left: '50%',
    transform: 'translate(-50%, -50%)',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: '2px',
  },
  centerValue: {
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  centerLabel: {
    fontSize: '9px',
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  legend: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    gap: '10px',
    minWidth: 0,
  },
  legendItem: {
    display: 'flex',
    flexDirection: 'column',
    gap: '4px',
  },
  legendHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
  },
  legendColor: {
    width: '10px',
    height: '10px',
    borderRadius: '2px',
    flexShrink: 0,
  },
  legendLabel: {
    fontSize: '11px',
    fontWeight: 500,
    color: '#c9d1d9',
    textTransform: 'capitalize',
    flex: 1,
  },
  legendPercent: {
    fontSize: '10px',
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  legendDetails: {
    display: 'flex',
    gap: '12px',
    paddingLeft: '18px',
  },
  legendDetailItem: {
    fontSize: '9px',
    color: '#6e7681',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  summary: {
    display: 'flex',
    justifyContent: 'space-around',
    paddingTop: '12px',
    borderTop: '1px solid #21262d',
  },
  summaryItem: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: '4px',
  },
  summaryLabel: {
    fontSize: '9px',
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  summaryValue: {
    fontSize: '13px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  empty: {
    fontSize: '11px',
    color: '#6e7681',
    fontStyle: 'italic',
    padding: '24px 0',
    textAlign: 'center',
  },
}

export default ProviderUsageChart
