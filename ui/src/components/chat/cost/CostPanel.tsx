/**
 * CostPanel Component
 *
 * Collapsible panel showing cost info, token usage, and budget progress.
 * Provides real-time cost tracking during agent execution.
 *
 * @module components/chat/cost/CostPanel
 */

import React from 'react'
import type { CostInfo } from '../../../types/chat'
import CostSummary from './CostSummary'
import TokenUsage from './TokenUsage'
import BudgetProgress from './BudgetProgress'
import { colors, typography, borderRadius } from '../../../utils/theme'
import { InfoIcon } from '../../../utils/icons'

export interface CostPanelProps {
  /** Cost tracking information */
  cost: CostInfo
  /** Provider name */
  provider?: string
  /** Model name */
  model?: string
  /** Context window size for token progress */
  contextWindowSize?: number
  /** Cache read tokens */
  cacheReadTokens?: number
  /** Cache write tokens */
  cacheWriteTokens?: number
}

const CostPanel: React.FC<CostPanelProps> = ({
  cost,
  provider,
  model,
  contextWindowSize,
  cacheReadTokens,
  cacheWriteTokens,
}) => {
  const hasBudget = cost.budgetLimit !== undefined && cost.budgetLimit > 0

  return (
    <div className="cost-panel" style={styles.container}>
      {/* Panel header */}
      <div className="cost-panel__header" style={styles.header}>
        <h3 style={styles.title}>Cost</h3>
        <span className="cost-panel__live" style={styles.live}>
          <span style={styles.liveDot} />
          Live
        </span>
      </div>

      {/* Cost summary section */}
      <div className="cost-panel__section" style={styles.section}>
        <CostSummary
          sessionTotal={cost.sessionTotal}
          iterationCost={cost.iterationCost}
          provider={provider}
          model={model}
        />
      </div>

      {/* Divider */}
      <div className="cost-panel__divider" style={styles.divider} />

      {/* Token usage section */}
      <div className="cost-panel__section" style={styles.section}>
        <h4 style={styles.sectionTitle}>Tokens</h4>
        <TokenUsage
          inputTokens={cost.totalTokensInput}
          outputTokens={cost.totalTokensOutput}
          cacheReadTokens={cacheReadTokens}
          cacheWriteTokens={cacheWriteTokens}
          maxTokens={contextWindowSize}
        />
      </div>

      {/* Budget section (if budget is set) */}
      {hasBudget && (
        <>
          <div className="cost-panel__divider" style={styles.divider} />
          <div className="cost-panel__section" style={styles.section}>
            <BudgetProgress
              used={cost.sessionTotal}
              limit={cost.budgetLimit!}
            />
          </div>
        </>
      )}

      {/* Cost breakdown tips */}
      <div className="cost-panel__tips" style={styles.tips}>
        <div className="cost-panel__tip" style={styles.tip}>
          <InfoIcon />
          <span>Costs are calculated using provider pricing APIs</span>
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
    height: '100%',
    backgroundColor: '#161b22',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '12px 12px 10px 12px',
    borderBottom: '1px solid #30363d',
    flexShrink: 0,
  },
  title: {
    margin: 0,
    fontSize: '14px',
    fontWeight: 600,
    color: '#c9d1d9',
  },
  live: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    backgroundColor: 'rgba(35, 134, 54, 0.2)',
    color: '#3fb950',
    fontSize: '10px',
    fontWeight: 500,
    padding: '2px 8px',
    borderRadius: '10px',
  },
  liveDot: {
    width: '6px',
    height: '6px',
    backgroundColor: '#3fb950',
    borderRadius: '50%',
  },
  section: {
    padding: '12px',
  },
  sectionTitle: {
    margin: '0 0 8px 0',
    fontSize: '11px',
    fontWeight: 600,
    color: '#8b949e',
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
  },
  divider: {
    height: '1px',
    backgroundColor: '#30363d',
    margin: '0 12px',
  },
  tips: {
    marginTop: 'auto',
    padding: '12px',
    borderTop: '1px solid #21262d',
  },
  tip: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: '8px',
    fontSize: '10px',
    color: '#6e7681',
    lineHeight: 1.4,
  },
}

export default CostPanel
