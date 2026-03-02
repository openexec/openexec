/**
 * TokenHistoryChart Component
 *
 * Displays a time-series line chart showing token usage over time.
 * Supports multiple series (input/output) with interactive tooltips.
 *
 * @module components/chat/cost/TokenHistoryChart
 */

import React, { useMemo, useState } from 'react'

export interface TokenHistoryDataPoint {
  /** Timestamp for the data point */
  timestamp: string
  /** Input tokens at this point */
  inputTokens: number
  /** Output tokens at this point */
  outputTokens: number
  /** Optional cost at this point */
  costUsd?: number
  /** Optional label (e.g., iteration number) */
  label?: string
}

export interface TokenHistoryChartProps {
  /** Time series data points */
  data: TokenHistoryDataPoint[]
  /** Title for the chart */
  title?: string
  /** Height of the chart area in pixels */
  height?: number
  /** Whether to show the cost line */
  showCost?: boolean
  /** Whether to show area fills under lines */
  showFill?: boolean
  /** Empty state message */
  emptyMessage?: string
  /** Maximum data points to display (rolling window) */
  maxPoints?: number
}

/** Series colors */
const SERIES_COLORS = {
  input: '#79c0ff',   // Blue for input
  output: '#7ee787',  // Green for output
  cost: '#ffd33d',    // Yellow for cost
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
  return count.toString()
}

/**
 * Format cost to USD string
 */
const formatCost = (cost: number): string => {
  if (cost === 0) return '$0.00'
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  return `$${cost.toFixed(2)}`
}

/**
 * Format timestamp for display
 */
const formatTime = (timestamp: string): string => {
  try {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    })
  } catch {
    return ''
  }
}

const TokenHistoryChart: React.FC<TokenHistoryChartProps> = ({
  data,
  title = 'Token Usage Over Time',
  height = 150,
  showCost = false,
  showFill = true,
  emptyMessage = 'No token history data',
  maxPoints = 50,
}) => {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)

  // Limit data to maxPoints (rolling window)
  const displayData = useMemo(() => {
    if (data.length <= maxPoints) return data
    return data.slice(data.length - maxPoints)
  }, [data, maxPoints])

  // Calculate chart dimensions and scales
  const chartConfig = useMemo(() => {
    const padding = { top: 20, right: 10, bottom: 30, left: 50 }
    const width = 400
    const chartWidth = width - padding.left - padding.right
    const chartHeight = height - padding.top - padding.bottom

    // Calculate max values for scaling
    const maxInputTokens = Math.max(...displayData.map(d => d.inputTokens), 1)
    const maxOutputTokens = Math.max(...displayData.map(d => d.outputTokens), 1)
    const maxTokens = Math.max(maxInputTokens, maxOutputTokens)

    // Calculate cost scale if needed
    const maxCost = showCost
      ? Math.max(...displayData.map(d => d.costUsd ?? 0), 0.001)
      : 0

    // X scale (even spacing)
    const xStep = displayData.length > 1
      ? chartWidth / (displayData.length - 1)
      : chartWidth

    return {
      padding,
      width,
      chartWidth,
      chartHeight,
      maxTokens,
      maxCost,
      xStep,
    }
  }, [displayData, height, showCost])

  // Generate path data for a series
  const generatePath = (
    values: number[],
    maxValue: number
  ): { linePath: string; areaPath: string } => {
    const { padding, chartHeight, xStep } = chartConfig
    const points: string[] = []

    values.forEach((value, index) => {
      const x = padding.left + index * xStep
      const y = padding.top + chartHeight - (value / maxValue) * chartHeight
      points.push(`${x},${y}`)
    })

    const linePath = `M ${points.join(' L ')}`

    // Area path (close at bottom)
    const firstX = padding.left
    const lastX = padding.left + (values.length - 1) * xStep
    const bottomY = padding.top + chartHeight
    const areaPath = `${linePath} L ${lastX},${bottomY} L ${firstX},${bottomY} Z`

    return { linePath, areaPath }
  }

  // Generate paths for each series
  const paths = useMemo(() => {
    const inputPath = generatePath(
      displayData.map(d => d.inputTokens),
      chartConfig.maxTokens
    )
    const outputPath = generatePath(
      displayData.map(d => d.outputTokens),
      chartConfig.maxTokens
    )
    const costPath = showCost
      ? generatePath(
          displayData.map(d => d.costUsd ?? 0),
          chartConfig.maxCost
        )
      : null

    return { inputPath, outputPath, costPath }
  }, [displayData, chartConfig, showCost])

  // Generate Y-axis ticks
  const yTicks = useMemo(() => {
    const { maxTokens, chartHeight, padding } = chartConfig
    const tickCount = 4
    const ticks: { value: number; y: number }[] = []

    for (let i = 0; i <= tickCount; i++) {
      const value = (maxTokens * i) / tickCount
      const y = padding.top + chartHeight - (value / maxTokens) * chartHeight
      ticks.push({ value, y })
    }

    return ticks
  }, [chartConfig])

  if (displayData.length === 0) {
    return (
      <div className="token-history-chart" style={styles.container}>
        {title && <h4 style={styles.title}>{title}</h4>}
        <div style={styles.empty}>{emptyMessage}</div>
      </div>
    )
  }

  const { padding, width, chartHeight } = chartConfig

  return (
    <div className="token-history-chart" style={styles.container}>
      {title && <h4 style={styles.title}>{title}</h4>}

      {/* Legend */}
      <div className="token-history-chart__legend" style={styles.legend}>
        <div style={styles.legendItem}>
          <span style={{ ...styles.legendColor, backgroundColor: SERIES_COLORS.input }} />
          <span style={styles.legendLabel}>Input</span>
        </div>
        <div style={styles.legendItem}>
          <span style={{ ...styles.legendColor, backgroundColor: SERIES_COLORS.output }} />
          <span style={styles.legendLabel}>Output</span>
        </div>
        {showCost && (
          <div style={styles.legendItem}>
            <span style={{ ...styles.legendColor, backgroundColor: SERIES_COLORS.cost }} />
            <span style={styles.legendLabel}>Cost</span>
          </div>
        )}
      </div>

      {/* SVG Chart */}
      <svg
        width="100%"
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="xMidYMid meet"
        className="token-history-chart__svg"
      >
        {/* Y-axis grid lines and labels */}
        {yTicks.map(({ value, y }) => (
          <g key={value}>
            <line
              x1={padding.left}
              y1={y}
              x2={width - padding.right}
              y2={y}
              stroke="#21262d"
              strokeDasharray="2,2"
            />
            <text
              x={padding.left - 5}
              y={y}
              textAnchor="end"
              dominantBaseline="middle"
              fill="#6e7681"
              fontSize="9"
              fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
            >
              {formatTokens(value)}
            </text>
          </g>
        ))}

        {/* Area fills */}
        {showFill && (
          <>
            <path
              d={paths.inputPath.areaPath}
              fill={SERIES_COLORS.input}
              fillOpacity={0.1}
            />
            <path
              d={paths.outputPath.areaPath}
              fill={SERIES_COLORS.output}
              fillOpacity={0.1}
            />
          </>
        )}

        {/* Lines */}
        <path
          d={paths.inputPath.linePath}
          stroke={SERIES_COLORS.input}
          strokeWidth={2}
          fill="none"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d={paths.outputPath.linePath}
          stroke={SERIES_COLORS.output}
          strokeWidth={2}
          fill="none"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        {paths.costPath && (
          <path
            d={paths.costPath.linePath}
            stroke={SERIES_COLORS.cost}
            strokeWidth={1.5}
            strokeDasharray="4,2"
            fill="none"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        )}

        {/* Data points (interactive) */}
        {displayData.map((point, index) => {
          const x = padding.left + index * chartConfig.xStep
          const inputY = padding.top + chartHeight - (point.inputTokens / chartConfig.maxTokens) * chartHeight
          const outputY = padding.top + chartHeight - (point.outputTokens / chartConfig.maxTokens) * chartHeight

          return (
            <g
              key={index}
              onMouseEnter={() => setHoveredIndex(index)}
              onMouseLeave={() => setHoveredIndex(null)}
              style={{ cursor: 'pointer' }}
            >
              {/* Invisible hitbox */}
              <rect
                x={x - chartConfig.xStep / 2}
                y={padding.top}
                width={chartConfig.xStep}
                height={chartHeight}
                fill="transparent"
              />

              {/* Hover line */}
              {hoveredIndex === index && (
                <line
                  x1={x}
                  y1={padding.top}
                  x2={x}
                  y2={padding.top + chartHeight}
                  stroke="#484f58"
                  strokeWidth={1}
                />
              )}

              {/* Data point markers on hover */}
              {hoveredIndex === index && (
                <>
                  <circle cx={x} cy={inputY} r={4} fill={SERIES_COLORS.input} />
                  <circle cx={x} cy={outputY} r={4} fill={SERIES_COLORS.output} />
                </>
              )}
            </g>
          )
        })}

        {/* X-axis labels (sample) */}
        {displayData.length <= 10
          ? displayData.map((point, index) => (
              <text
                key={index}
                x={padding.left + index * chartConfig.xStep}
                y={height - 5}
                textAnchor="middle"
                fill="#6e7681"
                fontSize="8"
                fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
              >
                {point.label ?? formatTime(point.timestamp)}
              </text>
            ))
          : // Show only first, middle, last
            [0, Math.floor(displayData.length / 2), displayData.length - 1].map(index => (
              <text
                key={index}
                x={padding.left + index * chartConfig.xStep}
                y={height - 5}
                textAnchor="middle"
                fill="#6e7681"
                fontSize="8"
                fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
              >
                {displayData[index].label ?? formatTime(displayData[index].timestamp)}
              </text>
            ))}
      </svg>

      {/* Tooltip */}
      {hoveredIndex !== null && displayData[hoveredIndex] && (
        <div
          className="token-history-chart__tooltip"
          style={styles.tooltip}
        >
          <div style={styles.tooltipHeader}>
            {displayData[hoveredIndex].label ??
              formatTime(displayData[hoveredIndex].timestamp)}
          </div>
          <div style={styles.tooltipRow}>
            <span style={{ ...styles.tooltipDot, backgroundColor: SERIES_COLORS.input }} />
            Input: {formatTokens(displayData[hoveredIndex].inputTokens)}
          </div>
          <div style={styles.tooltipRow}>
            <span style={{ ...styles.tooltipDot, backgroundColor: SERIES_COLORS.output }} />
            Output: {formatTokens(displayData[hoveredIndex].outputTokens)}
          </div>
          {showCost && displayData[hoveredIndex].costUsd !== undefined && (
            <div style={styles.tooltipRow}>
              <span style={{ ...styles.tooltipDot, backgroundColor: SERIES_COLORS.cost }} />
              Cost: {formatCost(displayData[hoveredIndex].costUsd)}
            </div>
          )}
        </div>
      )}

      {/* Summary stats */}
      <div className="token-history-chart__summary" style={styles.summary}>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Current Input</span>
          <span style={{ ...styles.summaryValue, color: SERIES_COLORS.input }}>
            {formatTokens(displayData[displayData.length - 1]?.inputTokens ?? 0)}
          </span>
        </div>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Current Output</span>
          <span style={{ ...styles.summaryValue, color: SERIES_COLORS.output }}>
            {formatTokens(displayData[displayData.length - 1]?.outputTokens ?? 0)}
          </span>
        </div>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Data Points</span>
          <span style={styles.summaryValue}>{displayData.length}</span>
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
    gap: '8px',
    position: 'relative',
  },
  title: {
    margin: 0,
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  legend: {
    display: 'flex',
    gap: '16px',
    justifyContent: 'center',
  },
  legendItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
  },
  legendColor: {
    width: '12px',
    height: '3px',
    borderRadius: '1px',
  },
  legendLabel: {
    fontSize: '10px',
    color: '#8b949e',
  },
  summary: {
    display: 'flex',
    justifyContent: 'space-around',
    paddingTop: '8px',
    borderTop: '1px solid #21262d',
  },
  summaryItem: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: '2px',
  },
  summaryLabel: {
    fontSize: '9px',
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  summaryValue: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  tooltip: {
    position: 'absolute',
    top: '40px',
    right: '10px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '6px',
    padding: '8px 12px',
    boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
    zIndex: 10,
  },
  tooltipHeader: {
    fontSize: '10px',
    fontWeight: 600,
    color: '#c9d1d9',
    marginBottom: '4px',
    paddingBottom: '4px',
    borderBottom: '1px solid #21262d',
  },
  tooltipRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '10px',
    color: '#8b949e',
    lineHeight: 1.6,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  tooltipDot: {
    width: '8px',
    height: '8px',
    borderRadius: '2px',
  },
  empty: {
    fontSize: '11px',
    color: '#6e7681',
    fontStyle: 'italic',
    padding: '40px 0',
    textAlign: 'center',
  },
}

export default TokenHistoryChart
