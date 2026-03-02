/**
 * Tests for ModelPickerWithAvailability component
 *
 * @module components/chat/header/__tests__/ModelPickerWithAvailability.test
 */

import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import ModelPickerWithAvailability from '../ModelPickerWithAvailability'
import { clearProviderAvailabilityCache } from '../../../../hooks/useProviderAvailability'
import type { ProviderInfo, ModelInfo } from '../../../../types/chat'

// =============================================================================
// Mock Data
// =============================================================================

const mockProviderStatus = [
  {
    name: 'openai',
    available: true,
    reason: 'configured',
    models: ['gpt-4o', 'gpt-4o-mini'],
  },
  {
    name: 'anthropic',
    available: true,
    reason: 'configured',
    models: ['claude-3-5-sonnet'],
  },
  {
    name: 'gemini',
    available: false,
    reason: 'API key not configured',
    models: [],
  },
]

const mockModels = [
  {
    id: 'gpt-4o',
    name: 'GPT-4o',
    provider: 'openai',
    contextWindow: 128000,
    maxOutputTokens: 16384,
    pricePerMInputTokens: 2.5,
    pricePerMOutputTokens: 10.0,
    supportsTools: true,
    supportsStreaming: true,
    supportsVision: true,
  },
  {
    id: 'gpt-4o-mini',
    name: 'GPT-4o Mini',
    provider: 'openai',
    contextWindow: 128000,
    maxOutputTokens: 16384,
    pricePerMInputTokens: 0.15,
    pricePerMOutputTokens: 0.6,
    supportsTools: true,
    supportsStreaming: true,
    supportsVision: true,
  },
  {
    id: 'claude-3-5-sonnet',
    name: 'Claude 3.5 Sonnet',
    provider: 'anthropic',
    contextWindow: 200000,
    maxOutputTokens: 8192,
    pricePerMInputTokens: 3.0,
    pricePerMOutputTokens: 15.0,
    supportsTools: true,
    supportsStreaming: true,
    supportsVision: true,
  },
]

const staticProviders: ProviderInfo[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    models: mockModels.filter((m) => m.provider === 'openai') as ModelInfo[],
    isAvailable: true,
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    models: mockModels.filter((m) => m.provider === 'anthropic') as ModelInfo[],
    isAvailable: true,
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    models: [],
    isAvailable: false,
  },
]

// =============================================================================
// Mock Setup
// =============================================================================

const mockFetch = vi.fn()

function createMockResponse<T>(data: T): Response {
  return {
    ok: true,
    status: 200,
    text: async () => JSON.stringify(data),
  } as Response
}

beforeEach(() => {
  clearProviderAvailabilityCache()
  vi.stubGlobal('fetch', mockFetch)
  mockFetch
    .mockResolvedValueOnce(createMockResponse(mockProviderStatus))
    .mockResolvedValueOnce(createMockResponse(mockModels))
})

afterEach(() => {
  vi.clearAllMocks()
  vi.unstubAllGlobals()
})

// =============================================================================
// Tests
// =============================================================================

describe('ModelPickerWithAvailability', () => {
  describe('rendering', () => {
    it('should render ModelPicker and ProviderAvailabilityIndicator', async () => {
      const onModelSelect = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Wait for data to load
      await waitFor(() => {
        // Should show the model picker trigger
        expect(screen.getByText('Select model...')).toBeInTheDocument()
      })

      // Should have provider availability indicators
      const dots = screen.getAllByTitle(/OpenAI|Anthropic|Google Gemini/i)
      expect(dots.length).toBeGreaterThan(0)
    })

    it('should show selected model in picker', async () => {
      const onModelSelect = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          selectedProvider="openai"
          selectedModel="gpt-4o"
          onModelSelect={onModelSelect}
        />
      )

      await waitFor(() => {
        expect(screen.getByText('GPT-4o')).toBeInTheDocument()
      })
    })
  })

  describe('model selection', () => {
    it('should call onModelSelect when a model from available provider is selected', async () => {
      const user = userEvent.setup()
      const onModelSelect = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText('Select model...')).toBeInTheDocument()
      })

      // Open the picker
      await user.click(screen.getByText('Select model...'))

      // Wait for dropdown
      await waitFor(() => {
        expect(screen.getByText('GPT-4o')).toBeInTheDocument()
      })

      // Select a model
      await user.click(screen.getByText('GPT-4o'))

      expect(onModelSelect).toHaveBeenCalledWith('openai', 'gpt-4o')
    })

    it('should not call onModelSelect when clicking model from unavailable provider', async () => {
      const user = userEvent.setup()
      const onModelSelect = vi.fn()

      // Create providers where gemini has models but is unavailable
      const providersWithUnavailableModels: ProviderInfo[] = [
        ...staticProviders.filter((p) => p.id !== 'gemini'),
        {
          id: 'gemini',
          name: 'Google Gemini',
          models: [
            {
              id: 'gemini-pro',
              name: 'Gemini Pro',
              provider: 'gemini',
              contextWindow: 1000000,
              maxOutputTokens: 8192,
              pricePerMInputTokens: 1.0,
              pricePerMOutputTokens: 2.0,
              supportsTools: true,
              supportsStreaming: true,
              supportsVision: true,
            },
          ],
          isAvailable: false,
          statusMessage: 'API key not configured',
        },
      ]

      // Mock unavailable gemini response
      mockFetch.mockReset()
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse([
            ...mockProviderStatus.filter((p) => p.name !== 'gemini'),
            { name: 'gemini', available: false, reason: 'API key not configured', models: [] },
          ])
        )
        .mockResolvedValueOnce(createMockResponse(mockModels))

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={providersWithUnavailableModels}
          onModelSelect={onModelSelect}
        />
      )

      // Wait for data to load
      await waitFor(() => {
        expect(screen.getByText('Select model...')).toBeInTheDocument()
      })

      // Open the picker
      await user.click(screen.getByText('Select model...'))

      // Wait for dropdown
      await waitFor(() => {
        expect(screen.getByText('Gemini Pro')).toBeInTheDocument()
      })

      // Try to select the unavailable model
      await user.click(screen.getByText('Gemini Pro'))

      // Should not have been called
      expect(onModelSelect).not.toHaveBeenCalled()
    })
  })

  describe('availability indicator', () => {
    it('should show availability indicator by default', async () => {
      const onModelSelect = vi.fn()

      const { container } = render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
        />
      )

      await waitFor(() => {
        const indicator = container.querySelector('.provider-availability')
        expect(indicator).toBeInTheDocument()
      })
    })

    it('should hide availability indicator when showAvailabilityIndicator is false', async () => {
      const onModelSelect = vi.fn()

      const { container } = render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
          showAvailabilityIndicator={false}
        />
      )

      await waitFor(() => {
        expect(screen.getByText('Select model...')).toBeInTheDocument()
      })

      const indicator = container.querySelector('.provider-availability')
      expect(indicator).not.toBeInTheDocument()
    })
  })

  describe('warning for unavailable selected provider', () => {
    it('should show warning when selected provider becomes unavailable', async () => {
      // Mock response where anthropic is unavailable
      mockFetch.mockReset()
      mockFetch
        .mockResolvedValueOnce(
          createMockResponse([
            { name: 'openai', available: true, reason: 'configured', models: [] },
            { name: 'anthropic', available: false, reason: 'Rate limited', models: [] },
            { name: 'gemini', available: false, reason: 'Not configured', models: [] },
          ])
        )
        .mockResolvedValueOnce(createMockResponse(mockModels))

      const onModelSelect = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          selectedProvider="anthropic"
          selectedModel="claude-3-5-sonnet"
          onModelSelect={onModelSelect}
        />
      )

      await waitFor(() => {
        expect(
          screen.getByText('Selected provider is currently unavailable')
        ).toBeInTheDocument()
      })
    })
  })

  describe('disabled state', () => {
    it('should disable picker when disabled prop is true', async () => {
      const onModelSelect = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
          disabled={true}
        />
      )

      await waitFor(() => {
        const trigger = screen.getByRole('button', { name: /select/i })
        expect(trigger).toBeDisabled()
      })
    })
  })

  describe('onAvailabilityChange callback', () => {
    it('should call onAvailabilityChange when providers are loaded', async () => {
      const onModelSelect = vi.fn()
      const onAvailabilityChange = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
          onAvailabilityChange={onAvailabilityChange}
        />
      )

      await waitFor(() => {
        expect(onAvailabilityChange).toHaveBeenCalled()
      })

      // Should be called with ProviderInfo array
      const callArg = onAvailabilityChange.mock.calls[0][0]
      expect(Array.isArray(callArg)).toBe(true)
      expect(callArg.length).toBeGreaterThan(0)
      expect(callArg[0]).toHaveProperty('id')
      expect(callArg[0]).toHaveProperty('isAvailable')
    })
  })

  describe('indicator mode', () => {
    it('should pass mode to availability indicator', async () => {
      const onModelSelect = vi.fn()

      render(
        <ModelPickerWithAvailability
          apiConfig={{ baseUrl: 'http://localhost:8080' }}
          staticProviders={staticProviders}
          onModelSelect={onModelSelect}
          availabilityIndicatorMode="expanded"
        />
      )

      await waitFor(() => {
        // In expanded mode, should show provider names
        expect(screen.getByText('OpenAI')).toBeInTheDocument()
      })
    })
  })
})
