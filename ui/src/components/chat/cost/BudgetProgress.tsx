/**
 * BudgetProgress Component
 *
 * Displays budget usage with progress bar and warning thresholds.
 *
 * @module components/chat/cost/BudgetProgress
 */

import React from 'react'

export interface BudgetProgressProps {
  /** Amount used in USD */
  used: number
  /** Budget limit in USD */
  limit: number
  /** Warning threshold percentage (default 75) */
  warnThreshold?: number
  /** Critical threshold percentage (default 90) */
  criticalThreshold?: number
}

/**
 * Format cost to USD string
 */
const formatCost = (cost: number): string => {
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  return `$${cost.toFixed(2)}`
}

const BudgetProgress: React.FC<BudgetProgressProps> = ({
  used,
  limit,
  warnThreshold = 75,
  criticalThreshold = 90,
}) => {
  const percentUsed = Math.min((used / limit) * 100, 100)
  const remaining = Math.max(limit - used, 0)
  const isOverBudget = used > limit

  // Determine color based on thresholds
  const getColor = (): string => {
    if (isOverBudget || percentUsed >= criticalThreshold) return '#f85149'
    if (percentUsed >= warnThreshold) return '#ffd33d'
    return '#3fb950'
  }

  // Determine status text
  const getStatus = (): { text: string; color: string } => {
    if (isOverBudget) return { text: 'Over budget', color: '#f85149' }
    if (percentUsed >= criticalThreshold) return { text: 'Critical', color: '#f85149' }
    if (percentUsed >= warnThreshold) return { text: 'Warning', color: '#ffd33d' }
    return { text: 'On track', color: '#3fb950' }
  }

  const color = getColor()
  const status = getStatus()

  return (
    <div className="budget-progress" style={styles.container}>
      {/* Header */}
      <div className="budget-progress__header" style={styles.header}>
        <span className="budget-progress__title" style={styles.title}>
          Budget
        </span>
        <span
          className="budget-progress__status"
          style={{ ...styles.status, color: status.color }}
        >
          {status.text}
        </span>
      </div>

      {/* Progress bar */}
      <div className="budget-progress__bar" style={styles.bar}>
        <div
          className="budget-progress__fill"
          style={{
            ...styles.fill,
            width: `${percentUsed}%`,
            backgroundColor: color,
          }}
        />
        {/* Warning threshold marker */}
        <div
          className="budget-progress__marker budget-progress__marker--warn"
          style={{
            ...styles.marker,
            left: `${warnThreshold}%`,
            backgroundColor: '#ffd33d',
          }}
          title={`Warning threshold (${warnThreshold}%)`}
        />
        {/* Critical threshold marker */}
        <div
          className="budget-progress__marker budget-progress__marker--critical"
          style={{
            ...styles.marker,
            left: `${criticalThreshold}%`,
            backgroundColor: '#f85149',
          }}
          title={`Critical threshold (${criticalThreshold}%)`}
        />
      </div>

      {/* Usage details */}
      <div className="budget-progress__details" style={styles.details}>
        <div className="budget-progress__used" style={styles.detailItem}>
          <span className="budget-progress__label" style={styles.label}>Used</span>
          <span
            className="budget-progress__value"
            style={{
              ...styles.value,
              color: isOverBudget ? '#f85149' : '#c9d1d9',
            }}
          >
            {formatCost(used)}
          </span>
        </div>
        <div className="budget-progress__remaining" style={styles.detailItem}>
          <span className="budget-progress__label" style={styles.label}>Remaining</span>
          <span
            className="budget-progress__value"
            style={{
              ...styles.value,
              color: remaining === 0 ? '#f85149' : '#3fb950',
            }}
          >
            {formatCost(remaining)}
          </span>
        </div>
        <div className="budget-progress__limit" style={styles.detailItem}>
          <span className="budget-progress__label" style={styles.label}>Limit</span>
          <span className="budget-progress__value" style={styles.value}>
            {formatCost(limit)}
          </span>
        </div>
      </div>

      {/* Percentage */}
      <div className="budget-progress__percent" style={styles.percent}>
        <span style={{ color }}>{percentUsed.toFixed(1)}%</span> of budget used
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
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  title: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  status: {
    fontSize: '10px',
    fontWeight: 500,
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  bar: {
    position: 'relative',
    height: '8px',
    backgroundColor: '#21262d',
    borderRadius: '4px',
    overflow: 'visible',
  },
  fill: {
    height: '100%',
    borderRadius: '4px',
    transition: 'width 0.3s ease, background-color 0.3s ease',
  },
  marker: {
    position: 'absolute',
    top: '-2px',
    width: '2px',
    height: '12px',
    borderRadius: '1px',
    transform: 'translateX(-1px)',
    opacity: 0.5,
  },
  details: {
    display: 'flex',
    justifyContent: 'space-between',
    gap: '16px',
  },
  detailItem: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  label: {
    fontSize: '10px',
    color: '#6e7681',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  value: {
    fontSize: '12px',
    fontWeight: 500,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
    color: '#c9d1d9',
  },
  percent: {
    fontSize: '11px',
    color: '#8b949e',
    textAlign: 'center',
  },
}

export default BudgetProgress
