/**
 * CostSummary Component
 *
 * Displays session and iteration cost totals in USD.
 *
 * @module components/chat/cost/CostSummary
 */

import React from 'react'

export interface CostSummaryProps {
  /** Total session cost in USD */
  sessionTotal: number
  /** Current iteration cost in USD */
  iterationCost?: number
  /** Provider name (for display) */
  provider?: string
  /** Model name (for display) */
  model?: string
  /** Size variant */
  size?: 'small' | 'medium' | 'large'
}

/**
 * Format cost to USD string
 */
const formatCost = (cost: number, precision: number = 4): string => {
  if (cost === 0) return '$0.00'
  if (cost < 0.0001) return '<$0.0001'
  if (cost < 0.01) return `$${cost.toFixed(precision)}`
  return `$${cost.toFixed(2)}`
}

const CostSummary: React.FC<CostSummaryProps> = ({
  sessionTotal,
  iterationCost,
  provider,
  model,
  size = 'medium',
}) => {
  const sizeStyles = {
    small: {
      totalSize: '18px',
      labelSize: '10px',
      detailSize: '11px',
    },
    medium: {
      totalSize: '24px',
      labelSize: '11px',
      detailSize: '12px',
    },
    large: {
      totalSize: '32px',
      labelSize: '12px',
      detailSize: '14px',
    },
  }

  const currentSize = sizeStyles[size]

  return (
    <div className="cost-summary" style={styles.container}>
      {/* Session total */}
      <div className="cost-summary__total" style={styles.totalSection}>
        <span className="cost-summary__label" style={{ ...styles.label, fontSize: currentSize.labelSize }}>
          Session Total
        </span>
        <span
          className="cost-summary__value"
          style={{
            ...styles.totalValue,
            fontSize: currentSize.totalSize,
          }}
        >
          {formatCost(sessionTotal)}
        </span>
      </div>

      {/* Iteration cost (if provided) */}
      {iterationCost !== undefined && iterationCost > 0 && (
        <div className="cost-summary__iteration" style={styles.iterationSection}>
          <span className="cost-summary__label" style={{ ...styles.label, fontSize: currentSize.labelSize }}>
            This Iteration
          </span>
          <span className="cost-summary__detail" style={{ ...styles.detailValue, fontSize: currentSize.detailSize }}>
            +{formatCost(iterationCost)}
          </span>
        </div>
      )}

      {/* Provider/Model info */}
      {(provider || model) && (
        <div className="cost-summary__model" style={styles.modelSection}>
          {provider && (
            <span className="cost-summary__provider" style={styles.provider}>
              {provider}
            </span>
          )}
          {model && (
            <span className="cost-summary__model-name" style={styles.modelName}>
              {model}
            </span>
          )}
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
    gap: '8px',
  },
  totalSection: {
    display: 'flex',
    flexDirection: 'column',
    gap: '2px',
  },
  label: {
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    fontWeight: 500,
  },
  totalValue: {
    color: '#c9d1d9',
    fontWeight: 600,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  iterationSection: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '6px 8px',
    backgroundColor: '#21262d',
    borderRadius: '4px',
  },
  detailValue: {
    color: '#3fb950',
    fontWeight: 500,
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  modelSection: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    marginTop: '4px',
  },
  provider: {
    fontSize: '10px',
    color: '#8b949e',
    backgroundColor: '#21262d',
    padding: '2px 6px',
    borderRadius: '3px',
    textTransform: 'uppercase',
  },
  modelName: {
    fontSize: '11px',
    color: '#6e7681',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
}

export default CostSummary
