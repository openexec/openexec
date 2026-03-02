/**
 * CostTimelineChart Component
 *
 * Displays a time-series area chart showing cumulative cost over iterations.
 * Includes iteration markers, cost accumulation line, and budget threshold indicators.
 *
 * @module components/chat/cost/CostTimelineChart
 */

import React, { useMemo, useState } from 'react'

export interface CostDataPoint {
  /** Iteration number or timestamp */
  iteration: number
  /** Timestamp for the data point */
  timestamp: string
  /** Cumulative cost at this point in USD */
  cumulativeCost: number
  /** Incremental cost for this iteration in USD */
  iterationCost: number
  /** Optional label (e.g., tool name, action) */
  label?: string
  /** Input tokens used in this iteration */
  inputTokens?: number
  /** Output tokens used in this iteration */
  outputTokens?: number
}

export interface CostTimelineChartProps {
  /** Time series data points */
  data: CostDataPoint[]
  /** Title for the chart */
  title?: string
  /** Height of the chart area in pixels */
  height?: number
  /** Budget limit for threshold line */
  budgetLimit?: number
  /** Warning threshold percentage (default: 75) */
  warningThreshold?: number
  /** Whether to show iteration cost bars */
  showIterationBars?: boolean
  /** Whether to show area fill under the line */
  showFill?: boolean
  /** Empty state message */
  emptyMessage?: string
  /** Maximum data points to display (rolling window) */
  maxPoints?: number
}

/** Colors for the chart */
const COLORS = {
  line: '#ffd33d',        // Yellow for cost line
  fill: 'rgba(255, 211, 61, 0.15)',
  iterationBar: '#79c0ff', // Blue for iteration cost bars
  budget: '#f85149',       // Red for budget line
  warning: '#ffa657',      // Orange for warning threshold
  grid: '#21262d',
  text: '#8b949e',
  textLight: '#6e7681',
}

/**
 * Format cost to USD string
 */
const formatCost = (cost: number): string => {
  if (cost === 0) return '$0.00'
  if (cost < 0.0001) return '<$0.0001'
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  if (cost < 1) return `$${cost.toFixed(3)}`
  return `$${cost.toFixed(2)}`
}

/**
 * Format time for display
 */
const formatTime = (timestamp: string): string => {
  try {
    const date = new Date(timestamp)
    return date.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return ''
  }
}

const CostTimelineChart: React.FC<CostTimelineChartProps> = ({
  data,
  title = 'Cost Over Time',
  height = 160,
  budgetLimit,
  warningThreshold = 75,
  showIterationBars = true,
  showFill = true,
  emptyMessage = 'No cost data available',
  maxPoints = 30,
}) => {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)

  // Limit data to maxPoints (rolling window)
  const displayData = useMemo(() => {
    if (data.length <= maxPoints) return data
    return data.slice(data.length - maxPoints)
  }, [data, maxPoints])

  // Calculate chart dimensions and scales
  const chartConfig = useMemo(() => {
    const padding = { top: 24, right: 12, bottom: 32, left: 55 }
    const width = 400
    const chartWidth = width - padding.left - padding.right
    const chartHeight = height - padding.top - padding.bottom

    // Calculate max values for scaling
    const maxCumulativeCost = Math.max(
      ...displayData.map(d => d.cumulativeCost),
      budgetLimit ?? 0,
      0.001
    )
    const maxIterationCost = Math.max(
      ...displayData.map(d => d.iterationCost),
      0.0001
    )

    // Add 10% headroom for visual clarity
    const yMax = maxCumulativeCost * 1.1

    // X scale (even spacing)
    const xStep = displayData.length > 1
      ? chartWidth / (displayData.length - 1)
      : chartWidth

    // Bar width for iteration costs
    const barWidth = Math.min(xStep * 0.6, 20)

    return {
      padding,
      width,
      chartWidth,
      chartHeight,
      maxCumulativeCost,
      maxIterationCost,
      yMax,
      xStep,
      barWidth,
    }
  }, [displayData, height, budgetLimit])

  // Generate path for cumulative cost line
  const costLinePath = useMemo(() => {
    const { padding, chartHeight, xStep, yMax } = chartConfig
    if (displayData.length === 0) return ''

    const points = displayData.map((d, index) => {
      const x = padding.left + index * xStep
      const y = padding.top + chartHeight - (d.cumulativeCost / yMax) * chartHeight
      return `${x},${y}`
    })

    return `M ${points.join(' L ')}`
  }, [displayData, chartConfig])

  // Generate area path for fill
  const areaPath = useMemo(() => {
    const { padding, chartHeight, xStep, yMax } = chartConfig
    if (displayData.length === 0) return ''

    const points = displayData.map((d, index) => {
      const x = padding.left + index * xStep
      const y = padding.top + chartHeight - (d.cumulativeCost / yMax) * chartHeight
      return `${x},${y}`
    })

    const firstX = padding.left
    const lastX = padding.left + (displayData.length - 1) * xStep
    const bottomY = padding.top + chartHeight

    return `M ${points.join(' L ')} L ${lastX},${bottomY} L ${firstX},${bottomY} Z`
  }, [displayData, chartConfig])

  // Generate Y-axis ticks
  const yTicks = useMemo(() => {
    const { yMax, chartHeight, padding } = chartConfig
    const tickCount = 4
    const ticks: { value: number; y: number }[] = []

    for (let i = 0; i <= tickCount; i++) {
      const value = (yMax * i) / tickCount
      const y = padding.top + chartHeight - (value / yMax) * chartHeight
      ticks.push({ value, y })
    }

    return ticks
  }, [chartConfig])

  // Budget threshold positions
  const budgetY = useMemo(() => {
    if (!budgetLimit) return null
    const { padding, chartHeight, yMax } = chartConfig
    return padding.top + chartHeight - (budgetLimit / yMax) * chartHeight
  }, [budgetLimit, chartConfig])

  const warningY = useMemo(() => {
    if (!budgetLimit) return null
    const warningValue = budgetLimit * (warningThreshold / 100)
    const { padding, chartHeight, yMax } = chartConfig
    return padding.top + chartHeight - (warningValue / yMax) * chartHeight
  }, [budgetLimit, warningThreshold, chartConfig])

  if (displayData.length === 0) {
    return (
      <div className="cost-timeline-chart" style={styles.container}>
        {title && <h4 style={styles.title}>{title}</h4>}
        <div style={styles.empty}>{emptyMessage}</div>
      </div>
    )
  }

  const { padding, width, chartHeight, xStep, barWidth, yMax, maxIterationCost } = chartConfig
  const currentCost = displayData[displayData.length - 1]?.cumulativeCost ?? 0
  const totalIterations = displayData.length

  return (
    <div className="cost-timeline-chart" style={styles.container}>
      {title && <h4 style={styles.title}>{title}</h4>}

      {/* Legend */}
      <div className="cost-timeline-chart__legend" style={styles.legend}>
        <div style={styles.legendItem}>
          <span style={{ ...styles.legendLine, backgroundColor: COLORS.line }} />
          <span style={styles.legendLabel}>Cumulative Cost</span>
        </div>
        {showIterationBars && (
          <div style={styles.legendItem}>
            <span style={{ ...styles.legendBox, backgroundColor: COLORS.iterationBar }} />
            <span style={styles.legendLabel}>Per Iteration</span>
          </div>
        )}
        {budgetLimit && (
          <div style={styles.legendItem}>
            <span style={{ ...styles.legendLine, backgroundColor: COLORS.budget, borderStyle: 'dashed' }} />
            <span style={styles.legendLabel}>Budget</span>
          </div>
        )}
      </div>

      {/* SVG Chart */}
      <svg
        width="100%"
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="xMidYMid meet"
        className="cost-timeline-chart__svg"
      >
        {/* Y-axis grid lines and labels */}
        {yTicks.map(({ value, y }) => (
          <g key={value}>
            <line
              x1={padding.left}
              y1={y}
              x2={width - padding.right}
              y2={y}
              stroke={COLORS.grid}
              strokeDasharray="2,2"
            />
            <text
              x={padding.left - 5}
              y={y}
              textAnchor="end"
              dominantBaseline="middle"
              fill={COLORS.textLight}
              fontSize="9"
              fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
            >
              {formatCost(value)}
            </text>
          </g>
        ))}

        {/* Warning threshold line */}
        {warningY !== null && (
          <line
            x1={padding.left}
            y1={warningY}
            x2={width - padding.right}
            y2={warningY}
            stroke={COLORS.warning}
            strokeWidth={1}
            strokeDasharray="4,4"
            opacity={0.6}
          />
        )}

        {/* Budget limit line */}
        {budgetY !== null && (
          <>
            <line
              x1={padding.left}
              y1={budgetY}
              x2={width - padding.right}
              y2={budgetY}
              stroke={COLORS.budget}
              strokeWidth={1.5}
              strokeDasharray="6,3"
            />
            <text
              x={width - padding.right}
              y={budgetY - 4}
              textAnchor="end"
              fill={COLORS.budget}
              fontSize="8"
              fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
            >
              Budget: {formatCost(budgetLimit!)}
            </text>
          </>
        )}

        {/* Iteration cost bars (behind the line) */}
        {showIterationBars && displayData.map((point, index) => {
          const x = padding.left + index * xStep - barWidth / 2
          const barHeight = maxIterationCost > 0
            ? (point.iterationCost / maxIterationCost) * (chartHeight * 0.5)
            : 0
          const y = padding.top + chartHeight - barHeight

          return (
            <rect
              key={`bar-${index}`}
              x={x}
              y={y}
              width={barWidth}
              height={barHeight}
              fill={COLORS.iterationBar}
              opacity={hoveredIndex === index ? 0.8 : 0.3}
              rx={2}
              ry={2}
            />
          )
        })}

        {/* Area fill */}
        {showFill && (
          <path
            d={areaPath}
            fill={COLORS.fill}
          />
        )}

        {/* Cumulative cost line */}
        <path
          d={costLinePath}
          stroke={COLORS.line}
          strokeWidth={2}
          fill="none"
          strokeLinecap="round"
          strokeLinejoin="round"
        />

        {/* Data point markers and hitboxes */}
        {displayData.map((point, index) => {
          const x = padding.left + index * xStep
          const y = padding.top + chartHeight - (point.cumulativeCost / yMax) * chartHeight

          return (
            <g
              key={index}
              onMouseEnter={() => setHoveredIndex(index)}
              onMouseLeave={() => setHoveredIndex(null)}
              style={{ cursor: 'pointer' }}
            >
              {/* Invisible hitbox */}
              <rect
                x={x - xStep / 2}
                y={padding.top}
                width={xStep}
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

              {/* Point marker */}
              <circle
                cx={x}
                cy={y}
                r={hoveredIndex === index ? 5 : 3}
                fill={COLORS.line}
                stroke="#161b22"
                strokeWidth={1.5}
              />
            </g>
          )
        })}

        {/* X-axis labels (iterations) */}
        {displayData.length <= 15
          ? displayData.map((point, index) => (
              <text
                key={index}
                x={padding.left + index * xStep}
                y={height - 8}
                textAnchor="middle"
                fill={COLORS.textLight}
                fontSize="8"
                fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
              >
                {point.label ?? `#${point.iteration}`}
              </text>
            ))
          : // Show only first, middle, last
            [0, Math.floor(displayData.length / 2), displayData.length - 1].map(index => (
              <text
                key={index}
                x={padding.left + index * xStep}
                y={height - 8}
                textAnchor="middle"
                fill={COLORS.textLight}
                fontSize="8"
                fontFamily="ui-monospace, SFMono-Regular, SF Mono, Menlo, monospace"
              >
                #{displayData[index].iteration}
              </text>
            ))}
      </svg>

      {/* Tooltip */}
      {hoveredIndex !== null && displayData[hoveredIndex] && (
        <div
          className="cost-timeline-chart__tooltip"
          style={styles.tooltip}
        >
          <div style={styles.tooltipHeader}>
            Iteration {displayData[hoveredIndex].iteration}
          </div>
          <div style={styles.tooltipRow}>
            <span style={{ ...styles.tooltipDot, backgroundColor: COLORS.line }} />
            Total: {formatCost(displayData[hoveredIndex].cumulativeCost)}
          </div>
          <div style={styles.tooltipRow}>
            <span style={{ ...styles.tooltipDot, backgroundColor: COLORS.iterationBar }} />
            This iter: {formatCost(displayData[hoveredIndex].iterationCost)}
          </div>
          {displayData[hoveredIndex].inputTokens !== undefined && (
            <div style={styles.tooltipRow}>
              Tokens: {displayData[hoveredIndex].inputTokens} in / {displayData[hoveredIndex].outputTokens} out
            </div>
          )}
          <div style={styles.tooltipTime}>
            {formatTime(displayData[hoveredIndex].timestamp)}
          </div>
        </div>
      )}

      {/* Summary stats */}
      <div className="cost-timeline-chart__summary" style={styles.summary}>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Current Cost</span>
          <span style={{ ...styles.summaryValue, color: COLORS.line }}>
            {formatCost(currentCost)}
          </span>
        </div>
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Iterations</span>
          <span style={styles.summaryValue}>{totalIterations}</span>
        </div>
        {budgetLimit && (
          <div style={styles.summaryItem}>
            <span style={styles.summaryLabel}>Budget Used</span>
            <span style={{
              ...styles.summaryValue,
              color: currentCost >= budgetLimit ? COLORS.budget :
                     currentCost >= budgetLimit * 0.75 ? COLORS.warning : '#7ee787'
            }}>
              {((currentCost / budgetLimit) * 100).toFixed(1)}%
            </span>
          </div>
        )}
        <div style={styles.summaryItem}>
          <span style={styles.summaryLabel}>Avg/Iteration</span>
          <span style={styles.summaryValue}>
            {formatCost(totalIterations > 0 ? currentCost / totalIterations : 0)}
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
    flexWrap: 'wrap',
  },
  legendItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
  },
  legendLine: {
    width: '16px',
    height: '3px',
    borderRadius: '1px',
  },
  legendBox: {
    width: '10px',
    height: '10px',
    borderRadius: '2px',
    opacity: 0.6,
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
    top: '50px',
    right: '10px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '6px',
    padding: '8px 12px',
    boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
    zIndex: 10,
    minWidth: '140px',
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
    flexShrink: 0,
  },
  tooltipTime: {
    fontSize: '9px',
    color: '#6e7681',
    marginTop: '4px',
    paddingTop: '4px',
    borderTop: '1px solid #21262d',
  },
  empty: {
    fontSize: '11px',
    color: '#6e7681',
    fontStyle: 'italic',
    padding: '40px 0',
    textAlign: 'center',
  },
}

export default CostTimelineChart
