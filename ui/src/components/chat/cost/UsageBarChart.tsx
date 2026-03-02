/**
 * UsageBarChart Component
 *
 * A lightweight SVG-based horizontal bar chart for displaying
 * usage metrics like provider cost breakdown or token usage by model.
 *
 * @module components/chat/cost/UsageBarChart
 */

import React from 'react'

export interface BarChartDataPoint {
  /** Label for the bar */
  label: string
  /** Value for the bar */
  value: number
  /** Optional color override */
  color?: string
  /** Optional sublabel (e.g., model name) */
  sublabel?: string
}

export interface UsageBarChartProps {
  /** Data points to display */
  data: BarChartDataPoint[]
  /** Title for the chart */
  title?: string
  /** Format function for values */
  formatValue?: (value: number) => string
  /** Height of each bar in pixels */
  barHeight?: number
  /** Gap between bars in pixels */
  barGap?: number
  /** Maximum value (auto-calculated if not provided) */
  maxValue?: number
  /** Color palette for bars (cycles through) */
  colors?: string[]
  /** Whether to show percentage labels */
  showPercentage?: boolean
  /** Empty state message */
  emptyMessage?: string
}

/** Default color palette matching GitHub dark theme */
const DEFAULT_COLORS = [
  '#79c0ff', // Blue
  '#7ee787', // Green
  '#d2a8ff', // Purple
  '#ff7b72', // Red
  '#ffd33d', // Yellow
  '#a5d6ff', // Light blue
  '#ffa657', // Orange
]

/**
 * Format number with K/M suffix
 */
const formatNumber = (value: number): string => {
  if (value >= 1000000) {
    return `${(value / 1000000).toFixed(1)}M`
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}K`
  }
  return value.toLocaleString()
}

/**
 * Format cost in USD
 */
const formatCost = (value: number): string => {
  if (value === 0) return '$0.00'
  if (value < 0.0001) return '<$0.0001'
  if (value < 0.01) return `$${value.toFixed(4)}`
  return `$${value.toFixed(2)}`
}

const UsageBarChart: React.FC<UsageBarChartProps> = ({
  data,
  title,
  formatValue = formatNumber,
  barHeight = 24,
  barGap = 8,
  maxValue,
  colors = DEFAULT_COLORS,
  showPercentage = true,
  emptyMessage = 'No data available',
}) => {
  // Calculate max value if not provided
  const calculatedMax = maxValue ?? Math.max(...data.map(d => d.value), 1)

  // Calculate total for percentages
  const total = data.reduce((sum, d) => sum + d.value, 0)

  // Calculate chart dimensions
  const labelWidth = 100
  const valueWidth = 80
  const chartWidth = 200
  const chartHeight = data.length * (barHeight + barGap) - barGap

  if (data.length === 0) {
    return (
      <div className="usage-bar-chart" style={styles.container}>
        {title && <h4 style={styles.title}>{title}</h4>}
        <div style={styles.empty}>{emptyMessage}</div>
      </div>
    )
  }

  return (
    <div className="usage-bar-chart" style={styles.container}>
      {title && <h4 style={styles.title}>{title}</h4>}

      <div className="usage-bar-chart__content" style={styles.content}>
        {data.map((item, index) => {
          const barWidth = calculatedMax > 0
            ? (item.value / calculatedMax) * 100
            : 0
          const percentage = total > 0
            ? (item.value / total) * 100
            : 0
          const color = item.color ?? colors[index % colors.length]

          return (
            <div
              key={item.label}
              className="usage-bar-chart__row"
              style={styles.row}
            >
              {/* Label column */}
              <div className="usage-bar-chart__label-col" style={styles.labelCol}>
                <span className="usage-bar-chart__label" style={styles.label}>
                  {item.label}
                </span>
                {item.sublabel && (
                  <span className="usage-bar-chart__sublabel" style={styles.sublabel}>
                    {item.sublabel}
                  </span>
                )}
              </div>

              {/* Bar column */}
              <div className="usage-bar-chart__bar-col" style={styles.barCol}>
                <div
                  className="usage-bar-chart__bar-bg"
                  style={styles.barBg}
                >
                  <div
                    className="usage-bar-chart__bar-fill"
                    style={{
                      ...styles.barFill,
                      width: `${Math.max(barWidth, 2)}%`,
                      backgroundColor: color,
                    }}
                  />
                </div>
              </div>

              {/* Value column */}
              <div className="usage-bar-chart__value-col" style={styles.valueCol}>
                <span className="usage-bar-chart__value" style={styles.value}>
                  {formatValue(item.value)}
                </span>
                {showPercentage && total > 0 && (
                  <span className="usage-bar-chart__percent" style={styles.percent}>
                    {percentage.toFixed(1)}%
                  </span>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* Total row */}
      {data.length > 1 && (
        <div className="usage-bar-chart__total" style={styles.totalRow}>
          <span style={styles.totalLabel}>Total</span>
          <span style={styles.totalValue}>{formatValue(total)}</span>
        </div>
      )}
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
    flexDirection: 'column',
    gap: '8px',
  },
  row: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    minHeight: '24px',
  },
  labelCol: {
    flex: '0 0 80px',
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
    overflow: 'hidden',
  },
  label: {
    fontSize: '11px',
    fontWeight: 500,
    color: '#c9d1d9',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  sublabel: {
    fontSize: '9px',
    color: '#6e7681',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  barCol: {
    flex: 1,
    minWidth: 0,
  },
  barBg: {
    height: '8px',
    backgroundColor: '#21262d',
    borderRadius: '4px',
    overflow: 'hidden',
  },
  barFill: {
    height: '100%',
    borderRadius: '4px',
    transition: 'width 0.3s ease',
    minWidth: '2px',
  },
  valueCol: {
    flex: '0 0 70px',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'flex-end',
    gap: '2px',
  },
  value: {
    fontSize: '11px',
    fontWeight: 500,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  percent: {
    fontSize: '9px',
    color: '#6e7681',
  },
  totalRow: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingTop: '8px',
    borderTop: '1px solid #21262d',
    marginTop: '4px',
  },
  totalLabel: {
    fontSize: '11px',
    fontWeight: 600,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  totalValue: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  empty: {
    fontSize: '11px',
    color: '#6e7681',
    fontStyle: 'italic',
    padding: '16px 0',
    textAlign: 'center',
  },
}

// Export helper formatters for reuse
export { formatNumber, formatCost }

export default UsageBarChart
