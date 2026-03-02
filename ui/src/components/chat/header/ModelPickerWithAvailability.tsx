/**
 * ModelPickerWithAvailability Component
 *
 * Combines the ModelPicker with real-time provider availability checking.
 * Provides:
 * - Automatic provider status fetching
 * - Real-time availability indicators
 * - Disabled models when provider unavailable
 * - Status messages in the picker dropdown
 *
 * @module components/chat/header/ModelPickerWithAvailability
 */
import React, { useCallback, useMemo } from 'react'
import ModelPicker, { type ModelPickerProps } from './ModelPicker'
import ProviderAvailabilityIndicator from './ProviderAvailabilityIndicator'
import { useProviderAvailability, type ProviderAvailabilityConfig } from '../../../hooks/useProviderAvailability'
import type { ProviderInfo } from '../../../types/chat'

// =============================================================================
// Types
// =============================================================================

export interface ModelPickerWithAvailabilityProps
  extends Omit<ModelPickerProps, 'providers'> {
  /** API configuration for availability checking */
  apiConfig: ProviderAvailabilityConfig
  /** Static provider/model data (optional, can be supplemented from API) */
  staticProviders?: ProviderInfo[]
  /** Show availability indicator next to the picker (default: true) */
  showAvailabilityIndicator?: boolean
  /** Availability indicator mode */
  availabilityIndicatorMode?: 'compact' | 'expanded'
  /** Callback when availability changes */
  onAvailabilityChange?: (providers: ProviderInfo[]) => void
}

// =============================================================================
// Component
// =============================================================================

const ModelPickerWithAvailability: React.FC<ModelPickerWithAvailabilityProps> = ({
  apiConfig,
  staticProviders = [],
  showAvailabilityIndicator = true,
  availabilityIndicatorMode = 'compact',
  onAvailabilityChange,
  selectedProvider,
  selectedModel,
  onModelSelect,
  disabled,
  showPricing,
  showCapabilities,
}) => {
  // Fetch provider availability
  const {
    providers: availabilityProviders,
    loading,
    error,
    lastUpdated,
    refresh,
    isProviderAvailable,
  } = useProviderAvailability(apiConfig)

  // Merge static providers with availability data
  const mergedProviders = useMemo((): ProviderInfo[] => {
    // If we have availability data from API, use it as the source of truth
    if (availabilityProviders.length > 0) {
      // Merge with static providers to supplement model data if needed
      return availabilityProviders.map((apiProvider) => {
        const staticProvider = staticProviders.find(
          (sp) => sp.id.toLowerCase() === apiProvider.id.toLowerCase()
        )

        return {
          ...apiProvider,
          // Use static models if API doesn't provide them
          models:
            apiProvider.models.length > 0
              ? apiProvider.models
              : staticProvider?.models ?? [],
        }
      })
    }

    // Fall back to static providers with inferred availability
    return staticProviders.map((provider) => ({
      ...provider,
      isAvailable: isProviderAvailable(provider.id) || provider.isAvailable,
    }))
  }, [availabilityProviders, staticProviders, isProviderAvailable])

  // Notify parent of availability changes
  React.useEffect(() => {
    if (onAvailabilityChange && mergedProviders.length > 0) {
      onAvailabilityChange(mergedProviders)
    }
  }, [mergedProviders, onAvailabilityChange])

  // Enhanced model selection that validates availability
  const handleModelSelect = useCallback(
    (provider: string, model: string) => {
      // Check if provider is available before allowing selection
      const providerInfo = mergedProviders.find(
        (p) => p.id.toLowerCase() === provider.toLowerCase()
      )

      if (!providerInfo?.isAvailable) {
        console.warn(
          `[ModelPickerWithAvailability] Cannot select model from unavailable provider: ${provider}`
        )
        return
      }

      onModelSelect(provider, model)
    },
    [mergedProviders, onModelSelect]
  )

  // Check if selected model's provider is still available
  const isSelectedProviderAvailable = useMemo(() => {
    if (!selectedProvider) return true
    return isProviderAvailable(selectedProvider)
  }, [selectedProvider, isProviderAvailable])

  return (
    <div className="model-picker-with-availability" style={styles.container}>
      {/* Provider Availability Indicator */}
      {showAvailabilityIndicator && (
        <ProviderAvailabilityIndicator
          providers={mergedProviders}
          loading={loading}
          error={error}
          lastUpdated={lastUpdated}
          onRefresh={refresh}
          mode={availabilityIndicatorMode}
          showRefresh={availabilityIndicatorMode === 'expanded'}
          showTimestamp={availabilityIndicatorMode === 'expanded'}
        />
      )}

      {/* Model Picker */}
      <div
        className="model-picker-with-availability__picker"
        style={{
          ...styles.pickerContainer,
          ...(loading ? styles.pickerLoading : {}),
        }}
      >
        <ModelPicker
          providers={mergedProviders}
          selectedProvider={selectedProvider}
          selectedModel={selectedModel}
          onModelSelect={handleModelSelect}
          disabled={disabled || loading}
          showPricing={showPricing}
          showCapabilities={showCapabilities}
        />

        {/* Warning if selected provider is no longer available */}
        {!isSelectedProviderAvailable && selectedProvider && (
          <div
            className="model-picker-with-availability__warning"
            style={styles.warning}
          >
            <WarningIcon />
            <span>Selected provider is currently unavailable</span>
          </div>
        )}
      </div>
    </div>
  )
}

// =============================================================================
// Icon Components
// =============================================================================

const WarningIcon: React.FC = () => (
  <svg
    width="14"
    height="14"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
  >
    <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
    <line x1="12" y1="9" x2="12" y2="13" />
    <line x1="12" y1="17" x2="12.01" y2="17" />
  </svg>
)

// =============================================================================
// Styles
// =============================================================================

const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    alignItems: 'center',
    gap: '12px',
  },
  pickerContainer: {
    position: 'relative',
    transition: 'opacity 0.15s ease',
  },
  pickerLoading: {
    opacity: 0.8,
  },
  warning: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    marginTop: '4px',
    padding: '4px 8px',
    fontSize: '11px',
    color: '#d29922',
    backgroundColor: '#d299220d',
    borderRadius: '4px',
  },
}

export default ModelPickerWithAvailability
