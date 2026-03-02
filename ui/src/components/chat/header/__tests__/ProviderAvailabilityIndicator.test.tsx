/**
 * Tests for ProviderAvailabilityIndicator component
 *
 * @module components/chat/header/__tests__/ProviderAvailabilityIndicator.test
 */

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import ProviderAvailabilityIndicator from '../ProviderAvailabilityIndicator'
import type { ProviderInfo, ModelInfo } from '../../../../types/chat'

// =============================================================================
// Mock Data
// =============================================================================

const mockModels: ModelInfo[] = [
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
]

const mockProviders: ProviderInfo[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    models: mockModels,
    isAvailable: true,
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    models: [
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
    ],
    isAvailable: true,
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    models: [],
    isAvailable: false,
    statusMessage: 'API key not configured',
  },
]

// =============================================================================
// Tests
// =============================================================================

describe('ProviderAvailabilityIndicator', () => {
  describe('rendering', () => {
    it('should render provider status dots', () => {
      render(<ProviderAvailabilityIndicator providers={mockProviders} />)

      // Should have 3 status dots
      const dots = screen.getAllByTitle(/OpenAI|Anthropic|Google Gemini/i)
      expect(dots).toHaveLength(3)
    })

    it('should show available status for configured providers', () => {
      render(<ProviderAvailabilityIndicator providers={mockProviders} />)

      const openaiDot = screen.getByTitle(/OpenAI: Available/i)
      expect(openaiDot).toBeInTheDocument()

      const anthropicDot = screen.getByTitle(/Anthropic: Available/i)
      expect(anthropicDot).toBeInTheDocument()
    })

    it('should show unavailable status with message for unconfigured providers', () => {
      render(<ProviderAvailabilityIndicator providers={mockProviders} />)

      const geminiDot = screen.getByTitle(/Google Gemini: API key not configured/i)
      expect(geminiDot).toBeInTheDocument()
    })

    it('should show summary count in compact mode', () => {
      render(
        <ProviderAvailabilityIndicator providers={mockProviders} mode="compact" />
      )

      // 2 out of 3 providers available
      expect(screen.getByText('2/3')).toBeInTheDocument()
    })

    it('should show provider names in expanded mode', () => {
      render(
        <ProviderAvailabilityIndicator providers={mockProviders} mode="expanded" />
      )

      expect(screen.getByText('OpenAI')).toBeInTheDocument()
      expect(screen.getByText('Anthropic')).toBeInTheDocument()
      expect(screen.getByText('Google Gemini')).toBeInTheDocument()
    })
  })

  describe('loading state', () => {
    it('should show loading indicator when loading', () => {
      const { container } = render(
        <ProviderAvailabilityIndicator providers={mockProviders} loading={true} />
      )

      // Check for loading spinner (SVG with animation)
      const loadingElement = container.querySelector('.provider-availability__loading')
      expect(loadingElement).toBeInTheDocument()
    })
  })

  describe('error state', () => {
    it('should show error indicator when error is provided', () => {
      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          error="Failed to fetch provider status"
        />
      )

      const errorElement = screen.getByTitle('Failed to fetch provider status')
      expect(errorElement).toBeInTheDocument()
    })
  })

  describe('timestamp', () => {
    it('should show timestamp in expanded mode by default', () => {
      const lastUpdated = new Date(Date.now() - 30000) // 30 seconds ago

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          lastUpdated={lastUpdated}
        />
      )

      expect(screen.getByText(/Updated 30s ago/i)).toBeInTheDocument()
    })

    it('should not show timestamp in compact mode by default', () => {
      const lastUpdated = new Date(Date.now() - 30000)

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="compact"
          lastUpdated={lastUpdated}
        />
      )

      expect(screen.queryByText(/Updated/i)).not.toBeInTheDocument()
    })

    it('should show timestamp when showTimestamp is true regardless of mode', () => {
      const lastUpdated = new Date(Date.now() - 30000)

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="compact"
          lastUpdated={lastUpdated}
          showTimestamp={true}
        />
      )

      expect(screen.getByText(/Updated 30s ago/i)).toBeInTheDocument()
    })

    it('should format "just now" for very recent updates', () => {
      const lastUpdated = new Date(Date.now() - 5000) // 5 seconds ago

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          lastUpdated={lastUpdated}
        />
      )

      expect(screen.getByText(/Updated just now/i)).toBeInTheDocument()
    })

    it('should format minutes correctly', () => {
      const lastUpdated = new Date(Date.now() - 180000) // 3 minutes ago

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          lastUpdated={lastUpdated}
        />
      )

      expect(screen.getByText(/Updated 3m ago/i)).toBeInTheDocument()
    })
  })

  describe('refresh button', () => {
    it('should show refresh button in expanded mode by default', () => {
      const onRefresh = vi.fn()

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          onRefresh={onRefresh}
        />
      )

      const refreshButton = screen.getByLabelText('Refresh provider availability')
      expect(refreshButton).toBeInTheDocument()
    })

    it('should not show refresh button in compact mode by default', () => {
      const onRefresh = vi.fn()

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="compact"
          onRefresh={onRefresh}
        />
      )

      expect(screen.queryByLabelText('Refresh provider availability')).not.toBeInTheDocument()
    })

    it('should call onRefresh when refresh button is clicked', async () => {
      const user = userEvent.setup()
      const onRefresh = vi.fn()

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          onRefresh={onRefresh}
        />
      )

      const refreshButton = screen.getByLabelText('Refresh provider availability')
      await user.click(refreshButton)

      expect(onRefresh).toHaveBeenCalledTimes(1)
    })

    it('should disable refresh button while loading', () => {
      const onRefresh = vi.fn()

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          onRefresh={onRefresh}
          loading={true}
        />
      )

      const refreshButton = screen.getByLabelText('Refresh provider availability')
      expect(refreshButton).toBeDisabled()
    })
  })

  describe('tooltip', () => {
    it('should show tooltip on provider hover', async () => {
      render(
        <ProviderAvailabilityIndicator providers={mockProviders} mode="compact" />
      )

      const openaiDot = screen.getByTitle(/OpenAI: Available/i)

      // Hover over the dot's container
      fireEvent.mouseEnter(openaiDot.parentElement!)

      await waitFor(() => {
        expect(screen.getByText('Available')).toBeInTheDocument()
      })
    })

    it('should show model count in tooltip for available providers', async () => {
      render(
        <ProviderAvailabilityIndicator providers={mockProviders} mode="compact" />
      )

      const openaiDot = screen.getByTitle(/OpenAI: Available/i)
      fireEvent.mouseEnter(openaiDot.parentElement!)

      await waitFor(() => {
        expect(screen.getByText('2 models available')).toBeInTheDocument()
      })
    })

    it('should show status message in tooltip for unavailable providers', async () => {
      render(
        <ProviderAvailabilityIndicator providers={mockProviders} mode="compact" />
      )

      const geminiDot = screen.getByTitle(/Google Gemini: API key not configured/i)
      fireEvent.mouseEnter(geminiDot.parentElement!)

      await waitFor(() => {
        expect(screen.getByText('API key not configured')).toBeInTheDocument()
      })
    })

    it('should hide tooltip on mouse leave', async () => {
      render(
        <ProviderAvailabilityIndicator providers={mockProviders} mode="compact" />
      )

      const openaiDot = screen.getByTitle(/OpenAI: Available/i)
      const container = openaiDot.parentElement!

      // Show tooltip
      fireEvent.mouseEnter(container)

      await waitFor(() => {
        expect(screen.getByText('Available')).toBeInTheDocument()
      })

      // Hide tooltip
      fireEvent.mouseLeave(container)

      await waitFor(() => {
        expect(screen.queryByText('Available')).not.toBeInTheDocument()
      })
    })
  })

  describe('empty state', () => {
    it('should render empty when no providers', () => {
      const { container } = render(
        <ProviderAvailabilityIndicator providers={[]} />
      )

      const indicators = container.querySelector('.provider-availability__indicators')
      expect(indicators?.children).toHaveLength(0)
    })

    it('should show 0/0 count when no providers in compact mode', () => {
      render(
        <ProviderAvailabilityIndicator providers={[]} mode="compact" />
      )

      expect(screen.getByText('0/0')).toBeInTheDocument()
    })
  })

  describe('all unavailable providers', () => {
    it('should handle all providers being unavailable', () => {
      const unavailableProviders: ProviderInfo[] = [
        {
          id: 'openai',
          name: 'OpenAI',
          models: [],
          isAvailable: false,
          statusMessage: 'API key not configured',
        },
        {
          id: 'anthropic',
          name: 'Anthropic',
          models: [],
          isAvailable: false,
          statusMessage: 'Rate limited',
        },
      ]

      render(
        <ProviderAvailabilityIndicator
          providers={unavailableProviders}
          mode="compact"
        />
      )

      expect(screen.getByText('0/2')).toBeInTheDocument()
    })
  })

  describe('accessibility', () => {
    it('should have accessible refresh button', () => {
      const onRefresh = vi.fn()

      render(
        <ProviderAvailabilityIndicator
          providers={mockProviders}
          mode="expanded"
          onRefresh={onRefresh}
        />
      )

      const refreshButton = screen.getByRole('button', {
        name: 'Refresh provider availability',
      })
      expect(refreshButton).toBeInTheDocument()
    })

    it('should have title attributes on status dots', () => {
      render(<ProviderAvailabilityIndicator providers={mockProviders} />)

      const dots = screen.getAllByTitle(/OpenAI|Anthropic|Google Gemini/i)
      dots.forEach((dot) => {
        expect(dot).toHaveAttribute('title')
      })
    })
  })
})
