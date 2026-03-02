/**
 * ModelIndicator Component
 * Shows current provider/model with status.
 * @module components/chat/header/ModelIndicator
 */
import React from 'react'

export interface ModelIndicatorProps {
  /** LLM provider name */
  provider: string
  /** Model ID or name */
  model: string
  /** Current status */
  status?: 'ready' | 'busy' | 'error'
}

const ModelIndicator: React.FC<ModelIndicatorProps> = ({
  provider,
  model,
  status = 'ready',
}) => {
  // Get status color
  const getStatusColor = (): string => {
    switch (status) {
      case 'ready':
        return '#238636'
      case 'busy':
        return '#f0883e'
      case 'error':
        return '#da3633'
      default:
        return '#8b949e'
    }
  }

  // Get display name for provider
  const getProviderDisplay = (): string => {
    const providerNames: Record<string, string> = {
      anthropic: 'Anthropic',
      openai: 'OpenAI',
      google: 'Google',
      gemini: 'Gemini',
    }
    return providerNames[provider.toLowerCase()] || provider
  }

  // Shorten model name for display
  const getModelDisplay = (): string => {
    // Common model name simplifications
    if (model.includes('claude-3-5-sonnet')) return 'Sonnet 3.5'
    if (model.includes('claude-3-opus')) return 'Opus 3'
    if (model.includes('claude-3-sonnet')) return 'Sonnet 3'
    if (model.includes('claude-3-haiku')) return 'Haiku 3'
    if (model.includes('gpt-4o')) return 'GPT-4o'
    if (model.includes('gpt-4-turbo')) return 'GPT-4 Turbo'
    if (model.includes('gpt-4')) return 'GPT-4'
    if (model.includes('gpt-3.5')) return 'GPT-3.5'
    if (model.includes('gemini-pro')) return 'Gemini Pro'
    if (model.includes('gemini-ultra')) return 'Gemini Ultra'
    // Return truncated model name if too long
    return model.length > 20 ? model.slice(0, 20) + '...' : model
  }

  return (
    <div className="model-indicator" style={styles.container}>
      {/* Status dot */}
      <span
        className="model-indicator__status"
        style={{
          ...styles.statusDot,
          backgroundColor: getStatusColor(),
        }}
        title={`Status: ${status}`}
      />

      {/* Provider */}
      <span className="model-indicator__provider" style={styles.provider}>
        {getProviderDisplay()}
      </span>

      {/* Separator */}
      <span className="model-indicator__separator" style={styles.separator}>
        /
      </span>

      {/* Model */}
      <span className="model-indicator__model" style={styles.model} title={model}>
        {getModelDisplay()}
      </span>
    </div>
  )
}

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    fontSize: '12px',
    backgroundColor: '#21262d',
    padding: '4px 8px',
    borderRadius: '4px',
  },
  statusDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  provider: {
    color: '#8b949e',
    fontWeight: 500,
  },
  separator: {
    color: '#484f58',
  },
  model: {
    color: '#c9d1d9',
    fontWeight: 500,
  },
}

export default ModelIndicator
