/**
 * ModelPicker Component
 *
 * A dropdown component for selecting AI provider and model.
 * Displays available models grouped by provider with pricing and capability info.
 *
 * @module components/chat/header/ModelPicker
 */
import React, { useState, useRef, useEffect, useCallback } from 'react'
import type { ProviderInfo, ModelInfo } from '../../../types/chat'

export interface ModelPickerProps {
  /** Currently selected provider ID */
  selectedProvider?: string
  /** Currently selected model ID */
  selectedModel?: string
  /** Available providers with their models */
  providers: ProviderInfo[]
  /** Callback when model is selected */
  onModelSelect: (provider: string, model: string) => void
  /** Whether the picker is disabled */
  disabled?: boolean
  /** Show pricing information */
  showPricing?: boolean
  /** Show capability badges */
  showCapabilities?: boolean
}

/**
 * Format price per million tokens for display
 */
const formatPrice = (price: number): string => {
  if (price === 0) return 'Free'
  if (price < 0.01) return `$${price.toFixed(4)}/M`
  if (price < 1) return `$${price.toFixed(3)}/M`
  return `$${price.toFixed(2)}/M`
}

/**
 * Get display name for provider
 */
const getProviderDisplayName = (providerId: string): string => {
  const providerNames: Record<string, string> = {
    anthropic: 'Anthropic',
    openai: 'OpenAI',
    gemini: 'Google Gemini',
    google: 'Google Gemini',
  }
  return providerNames[providerId.toLowerCase()] || providerId
}

/**
 * Get icon/color for provider
 */
const getProviderColor = (providerId: string): string => {
  const colors: Record<string, string> = {
    anthropic: '#d97757',
    openai: '#10a37f',
    gemini: '#4285f4',
    google: '#4285f4',
  }
  return colors[providerId.toLowerCase()] || '#8b949e'
}

const ModelPicker: React.FC<ModelPickerProps> = ({
  selectedProvider,
  selectedModel,
  providers,
  onModelSelect,
  disabled = false,
  showPricing = true,
  showCapabilities = true,
}) => {
  const [isOpen, setIsOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false)
        setSearchQuery('')
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // Focus search input when dropdown opens
  useEffect(() => {
    if (isOpen && searchInputRef.current) {
      searchInputRef.current.focus()
    }
  }, [isOpen])

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Escape') {
        setIsOpen(false)
        setSearchQuery('')
      }
    },
    []
  )

  // Get currently selected model info
  const getSelectedModelInfo = (): ModelInfo | undefined => {
    if (!selectedProvider || !selectedModel) return undefined
    const provider = providers.find((p) => p.id === selectedProvider)
    return provider?.models.find((m) => m.id === selectedModel)
  }

  // Filter models based on search query
  const filterModels = (models: ModelInfo[]): ModelInfo[] => {
    if (!searchQuery) return models
    const query = searchQuery.toLowerCase()
    return models.filter(
      (m) =>
        m.name.toLowerCase().includes(query) ||
        m.id.toLowerCase().includes(query)
    )
  }

  // Check if provider has matching models
  const hasMatchingModels = (provider: ProviderInfo): boolean => {
    return filterModels(provider.models).length > 0
  }

  const selectedModelInfo = getSelectedModelInfo()

  // Handle model selection
  const handleModelSelect = (provider: ProviderInfo, model: ModelInfo) => {
    if (!provider.isAvailable) return
    onModelSelect(provider.id, model.id)
    setIsOpen(false)
    setSearchQuery('')
  }

  return (
    <div
      ref={containerRef}
      className="model-picker"
      style={styles.container}
      onKeyDown={handleKeyDown}
    >
      {/* Trigger Button */}
      <button
        className="model-picker__trigger"
        style={{
          ...styles.trigger,
          ...(disabled ? styles.triggerDisabled : {}),
          ...(isOpen ? styles.triggerOpen : {}),
        }}
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        title={selectedModelInfo ? `${selectedModelInfo.provider} / ${selectedModelInfo.name}` : 'Select a model'}
      >
        {selectedModelInfo ? (
          <>
            <span
              className="model-picker__provider-dot"
              style={{
                ...styles.providerDot,
                backgroundColor: getProviderColor(selectedModelInfo.provider),
              }}
            />
            <span className="model-picker__selected-text" style={styles.selectedText}>
              <span style={styles.selectedProvider}>
                {getProviderDisplayName(selectedModelInfo.provider)}
              </span>
              <span style={styles.selectedSeparator}>/</span>
              <span style={styles.selectedModel}>{selectedModelInfo.name}</span>
            </span>
          </>
        ) : (
          <span style={styles.placeholder}>Select model...</span>
        )}
        <ChevronDownIcon isOpen={isOpen} />
      </button>

      {/* Dropdown */}
      {isOpen && (
        <div className="model-picker__dropdown" style={styles.dropdown}>
          {/* Search Input */}
          <div className="model-picker__search-container" style={styles.searchContainer}>
            <SearchIcon />
            <input
              ref={searchInputRef}
              type="text"
              className="model-picker__search"
              style={styles.searchInput}
              placeholder="Search models..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              aria-label="Search models"
            />
            {searchQuery && (
              <button
                className="model-picker__search-clear"
                style={styles.searchClear}
                onClick={() => setSearchQuery('')}
                aria-label="Clear search"
              >
                <ClearIcon />
              </button>
            )}
          </div>

          {/* Provider Groups */}
          <div className="model-picker__list" style={styles.list} role="listbox">
            {providers.map((provider) => {
              const filteredModels = filterModels(provider.models)
              if (filteredModels.length === 0) return null

              return (
                <div key={provider.id} className="model-picker__provider-group">
                  {/* Provider Header */}
                  <div
                    className="model-picker__provider-header"
                    style={{
                      ...styles.providerHeader,
                      ...(!provider.isAvailable ? styles.providerHeaderDisabled : {}),
                    }}
                  >
                    <span
                      className="model-picker__provider-indicator"
                      style={{
                        ...styles.providerIndicator,
                        backgroundColor: provider.isAvailable
                          ? getProviderColor(provider.id)
                          : '#484f58',
                      }}
                    />
                    <span style={styles.providerName}>
                      {getProviderDisplayName(provider.id)}
                    </span>
                    {!provider.isAvailable && (
                      <span style={styles.providerStatus}>
                        {provider.statusMessage || 'Unavailable'}
                      </span>
                    )}
                  </div>

                  {/* Models */}
                  {filteredModels.map((model) => {
                    const isSelected =
                      selectedProvider === provider.id && selectedModel === model.id
                    const isDisabled = !provider.isAvailable

                    return (
                      <div
                        key={model.id}
                        className="model-picker__model-item"
                        style={{
                          ...styles.modelItem,
                          ...(isSelected ? styles.modelItemSelected : {}),
                          ...(isDisabled ? styles.modelItemDisabled : {}),
                        }}
                        onClick={() => handleModelSelect(provider, model)}
                        role="option"
                        aria-selected={isSelected}
                        aria-disabled={isDisabled}
                        tabIndex={isDisabled ? -1 : 0}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault()
                            handleModelSelect(provider, model)
                          }
                        }}
                      >
                        <div style={styles.modelMain}>
                          <span style={styles.modelName}>{model.name}</span>

                          {/* Capability Badges */}
                          {showCapabilities && (
                            <div style={styles.capabilities}>
                              {model.supportsVision && (
                                <span style={styles.capabilityBadge} title="Vision">
                                  <VisionIcon />
                                </span>
                              )}
                              {model.supportsTools && (
                                <span style={styles.capabilityBadge} title="Tool Use">
                                  <ToolIcon />
                                </span>
                              )}
                              {model.supportsStreaming && (
                                <span style={styles.capabilityBadge} title="Streaming">
                                  <StreamIcon />
                                </span>
                              )}
                            </div>
                          )}
                        </div>

                        <div style={styles.modelMeta}>
                          {/* Context Window */}
                          <span style={styles.contextWindow} title="Context window">
                            {formatContextWindow(model.contextWindow)}
                          </span>

                          {/* Pricing */}
                          {showPricing && (
                            <span style={styles.pricing}>
                              <span style={styles.pricingIn} title="Input price">
                                {formatPrice(model.pricePerMInputTokens)}
                              </span>
                              <span style={styles.pricingSeparator}>/</span>
                              <span style={styles.pricingOut} title="Output price">
                                {formatPrice(model.pricePerMOutputTokens)}
                              </span>
                            </span>
                          )}
                        </div>

                        {/* Selection indicator */}
                        {isSelected && (
                          <span style={styles.checkmark}>
                            <CheckIcon />
                          </span>
                        )}
                      </div>
                    )
                  })}
                </div>
              )
            })}

            {/* No results message */}
            {!providers.some((p) => hasMatchingModels(p)) && (
              <div style={styles.noResults}>
                No models found matching "{searchQuery}"
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * Format context window size for display
 */
const formatContextWindow = (tokens: number): string => {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`
  return tokens.toString()
}

// Icon Components
const ChevronDownIcon: React.FC<{ isOpen: boolean }> = ({ isOpen }) => (
  <svg
    width="12"
    height="12"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    style={{
      transition: 'transform 0.15s ease',
      transform: isOpen ? 'rotate(180deg)' : 'rotate(0deg)',
    }}
  >
    <polyline points="6 9 12 15 18 9" />
  </svg>
)

const SearchIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#8b949e" strokeWidth="2">
    <circle cx="11" cy="11" r="8" />
    <line x1="21" y1="21" x2="16.65" y2="16.65" />
  </svg>
)

const ClearIcon: React.FC = () => (
  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  </svg>
)

const CheckIcon: React.FC = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#58a6ff" strokeWidth="2.5">
    <polyline points="20 6 9 17 4 12" />
  </svg>
)

const VisionIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
    <path d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z" />
  </svg>
)

const ToolIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
    <path d="M22.7 19l-9.1-9.1c.9-2.3.4-5-1.5-6.9-2-2-5-2.4-7.4-1.3L9 6 6 9 1.6 4.7C.4 7.1.9 10.1 2.9 12.1c1.9 1.9 4.6 2.4 6.9 1.5l9.1 9.1c.4.4 1 .4 1.4 0l2.3-2.3c.5-.4.5-1.1.1-1.4z" />
  </svg>
)

const StreamIcon: React.FC = () => (
  <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
    <path d="M4 6h16v2H4zm0 5h16v2H4zm0 5h16v2H4z" />
  </svg>
)

// Styles
const styles: Record<string, React.CSSProperties> = {
  container: {
    position: 'relative',
    display: 'inline-block',
  },
  trigger: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '6px 10px',
    fontSize: '12px',
    fontWeight: 500,
    color: '#c9d1d9',
    backgroundColor: '#21262d',
    border: '1px solid #30363d',
    borderRadius: '6px',
    cursor: 'pointer',
    transition: 'border-color 0.15s ease, background-color 0.15s ease',
    minWidth: '160px',
  },
  triggerDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
  triggerOpen: {
    borderColor: '#58a6ff',
    backgroundColor: '#161b22',
  },
  providerDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  selectedText: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    flex: 1,
    overflow: 'hidden',
  },
  selectedProvider: {
    color: '#8b949e',
  },
  selectedSeparator: {
    color: '#484f58',
  },
  selectedModel: {
    color: '#c9d1d9',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  placeholder: {
    color: '#8b949e',
    fontStyle: 'italic',
  },
  dropdown: {
    position: 'absolute',
    top: '100%',
    left: 0,
    marginTop: '4px',
    minWidth: '320px',
    maxWidth: '400px',
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '8px',
    boxShadow: '0 8px 24px rgba(0, 0, 0, 0.4)',
    zIndex: 1000,
    overflow: 'hidden',
  },
  searchContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 12px',
    borderBottom: '1px solid #30363d',
  },
  searchInput: {
    flex: 1,
    background: 'transparent',
    border: 'none',
    outline: 'none',
    color: '#c9d1d9',
    fontSize: '13px',
  },
  searchClear: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '2px',
    background: 'transparent',
    border: 'none',
    cursor: 'pointer',
    color: '#8b949e',
  },
  list: {
    maxHeight: '400px',
    overflowY: 'auto',
    padding: '4px 0',
  },
  providerHeader: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 12px 4px',
    fontSize: '11px',
    fontWeight: 600,
    textTransform: 'uppercase',
    letterSpacing: '0.5px',
    color: '#8b949e',
  },
  providerHeaderDisabled: {
    opacity: 0.6,
  },
  providerIndicator: {
    width: '6px',
    height: '6px',
    borderRadius: '50%',
  },
  providerName: {
    flex: 1,
  },
  providerStatus: {
    fontSize: '10px',
    fontWeight: 400,
    color: '#da3633',
    textTransform: 'none',
  },
  modelItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    padding: '8px 12px 8px 26px',
    fontSize: '13px',
    cursor: 'pointer',
    transition: 'background-color 0.1s ease',
  },
  modelItemSelected: {
    backgroundColor: '#1f6feb20',
  },
  modelItemDisabled: {
    opacity: 0.5,
    cursor: 'not-allowed',
  },
  modelMain: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    flex: 1,
    minWidth: 0,
  },
  modelName: {
    color: '#c9d1d9',
    fontWeight: 500,
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  capabilities: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
  },
  capabilityBadge: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '16px',
    height: '16px',
    borderRadius: '3px',
    backgroundColor: '#21262d',
    color: '#8b949e',
  },
  modelMeta: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
    flexShrink: 0,
  },
  contextWindow: {
    fontSize: '11px',
    color: '#8b949e',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  pricing: {
    display: 'flex',
    alignItems: 'center',
    gap: '2px',
    fontSize: '10px',
    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
  },
  pricingIn: {
    color: '#3fb950',
  },
  pricingSeparator: {
    color: '#484f58',
  },
  pricingOut: {
    color: '#f0883e',
  },
  checkmark: {
    display: 'flex',
    alignItems: 'center',
    flexShrink: 0,
  },
  noResults: {
    padding: '16px',
    textAlign: 'center',
    color: '#8b949e',
    fontSize: '13px',
  },
}

export default ModelPicker
