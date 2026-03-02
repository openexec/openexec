/**
 * Tests for ModelPicker component
 *
 * @module components/chat/header/__tests__/ModelPicker.test
 */

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import ModelPicker from '../ModelPicker'
import type { ProviderInfo, ModelInfo } from '../../../../types/chat'

// Sample test data
const mockModels: Record<string, ModelInfo[]> = {
  anthropic: [
    {
      id: 'claude-3-5-sonnet-20241022',
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
    {
      id: 'claude-3-haiku-20240307',
      name: 'Claude 3 Haiku',
      provider: 'anthropic',
      contextWindow: 200000,
      maxOutputTokens: 4096,
      pricePerMInputTokens: 0.25,
      pricePerMOutputTokens: 1.25,
      supportsTools: true,
      supportsStreaming: true,
      supportsVision: true,
    },
  ],
  openai: [
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
  ],
  gemini: [
    {
      id: 'gemini-1.5-pro',
      name: 'Gemini 1.5 Pro',
      provider: 'gemini',
      contextWindow: 2097152,
      maxOutputTokens: 8192,
      pricePerMInputTokens: 1.25,
      pricePerMOutputTokens: 5.0,
      supportsTools: true,
      supportsStreaming: true,
      supportsVision: true,
    },
  ],
}

const mockProviders: ProviderInfo[] = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    models: mockModels.anthropic,
    isAvailable: true,
  },
  {
    id: 'openai',
    name: 'OpenAI',
    models: mockModels.openai,
    isAvailable: true,
  },
  {
    id: 'gemini',
    name: 'Google Gemini',
    models: mockModels.gemini,
    isAvailable: true,
  },
]

describe('ModelPicker', () => {
  let onModelSelect: ReturnType<typeof vi.fn>

  beforeEach(() => {
    onModelSelect = vi.fn()
  })

  describe('rendering', () => {
    it('should render with placeholder when no model is selected', () => {
      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      expect(screen.getByText('Select model...')).toBeInTheDocument()
    })

    it('should render with selected model displayed', () => {
      render(
        <ModelPicker
          providers={mockProviders}
          selectedProvider="anthropic"
          selectedModel="claude-3-5-sonnet-20241022"
          onModelSelect={onModelSelect}
        />
      )

      expect(screen.getByText('Anthropic')).toBeInTheDocument()
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()
    })

    it('should render disabled state correctly', () => {
      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
          disabled={true}
        />
      )

      const trigger = screen.getByRole('button')
      expect(trigger).toBeDisabled()
    })
  })

  describe('dropdown interaction', () => {
    it('should open dropdown when clicking trigger', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show provider headers (text-transform: uppercase is applied via CSS)
      expect(screen.getByText('Anthropic')).toBeInTheDocument()
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
      expect(screen.getByText('Google Gemini')).toBeInTheDocument()
    })

    it('should show all models when dropdown is open', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show all models
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()
      expect(screen.getByText('Claude 3 Haiku')).toBeInTheDocument()
      expect(screen.getByText('GPT-4o')).toBeInTheDocument()
      expect(screen.getByText('GPT-4o Mini')).toBeInTheDocument()
      expect(screen.getByText('Gemini 1.5 Pro')).toBeInTheDocument()
    })

    it('should close dropdown when clicking outside', async () => {
      const user = userEvent.setup()

      render(
        <div>
          <div data-testid="outside">Outside</div>
          <ModelPicker
            providers={mockProviders}
            onModelSelect={onModelSelect}
          />
        </div>
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()

      // Click outside
      fireEvent.mouseDown(screen.getByTestId('outside'))

      // Dropdown should be closed
      await waitFor(() => {
        expect(screen.queryByText('Claude 3.5 Sonnet')).not.toBeInTheDocument()
      })
    })

    it('should close dropdown on Escape key', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()

      // Press Escape
      await user.keyboard('{Escape}')

      // Dropdown should be closed
      await waitFor(() => {
        expect(screen.queryByText('Claude 3.5 Sonnet')).not.toBeInTheDocument()
      })
    })
  })

  describe('model selection', () => {
    it('should call onModelSelect when a model is clicked', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Click on a model
      const modelOption = screen.getByText('Claude 3.5 Sonnet')
      await user.click(modelOption)

      expect(onModelSelect).toHaveBeenCalledWith('anthropic', 'claude-3-5-sonnet-20241022')
    })

    it('should close dropdown after selection', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Click on a model
      const modelOption = screen.getByText('GPT-4o')
      await user.click(modelOption)

      // Dropdown should be closed
      await waitFor(() => {
        expect(screen.queryByText('Claude 3.5 Sonnet')).not.toBeInTheDocument()
      })
    })

    it('should show checkmark on selected model', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          selectedProvider="openai"
          selectedModel="gpt-4o"
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Find the selected model item by checking aria-selected attribute
      const options = screen.getAllByRole('option')
      const selectedOption = options.find((opt) => opt.getAttribute('aria-selected') === 'true')
      expect(selectedOption).toBeInTheDocument()
      // The selected option should contain GPT-4o text
      expect(selectedOption).toHaveTextContent('GPT-4o')
    })
  })

  describe('search functionality', () => {
    it('should filter models based on search query', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Type in search
      const searchInput = screen.getByPlaceholderText('Search models...')
      await user.type(searchInput, 'claude')

      // Should show only Claude models
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()
      expect(screen.getByText('Claude 3 Haiku')).toBeInTheDocument()

      // Should not show GPT models
      expect(screen.queryByText('GPT-4o')).not.toBeInTheDocument()
      expect(screen.queryByText('GPT-4o Mini')).not.toBeInTheDocument()
    })

    it('should show no results message when no models match', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Type a non-matching query
      const searchInput = screen.getByPlaceholderText('Search models...')
      await user.type(searchInput, 'nonexistent')

      // Should show no results message
      expect(screen.getByText(/No models found matching/)).toBeInTheDocument()
    })

    it('should clear search when clear button is clicked', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Type in search
      const searchInput = screen.getByPlaceholderText('Search models...')
      await user.type(searchInput, 'claude')

      // Click clear button
      const clearButton = screen.getByLabelText('Clear search')
      await user.click(clearButton)

      // Search should be cleared and all models should be visible
      expect(searchInput).toHaveValue('')
      expect(screen.getByText('GPT-4o')).toBeInTheDocument()
    })
  })

  describe('unavailable providers', () => {
    it('should show unavailable status for disabled providers', async () => {
      const user = userEvent.setup()

      const providersWithUnavailable: ProviderInfo[] = [
        ...mockProviders,
        {
          id: 'disabled-provider',
          name: 'Disabled Provider',
          models: [{
            id: 'disabled-model',
            name: 'Disabled Model',
            provider: 'disabled-provider',
            contextWindow: 100000,
            maxOutputTokens: 4096,
            pricePerMInputTokens: 1.0,
            pricePerMOutputTokens: 2.0,
            supportsTools: false,
            supportsStreaming: false,
          }],
          isAvailable: false,
          statusMessage: 'API key not configured',
        },
      ]

      render(
        <ModelPicker
          providers={providersWithUnavailable}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show unavailable status
      expect(screen.getByText('API key not configured')).toBeInTheDocument()
    })

    it('should not call onModelSelect for unavailable provider models', async () => {
      const user = userEvent.setup()

      const providersWithUnavailable: ProviderInfo[] = [
        {
          id: 'anthropic',
          name: 'Anthropic',
          models: mockModels.anthropic,
          isAvailable: false,
          statusMessage: 'Unavailable',
        },
      ]

      render(
        <ModelPicker
          providers={providersWithUnavailable}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Try to click on a model from unavailable provider
      const modelOption = screen.getByText('Claude 3.5 Sonnet')
      await user.click(modelOption)

      // Should not have called onModelSelect
      expect(onModelSelect).not.toHaveBeenCalled()
    })
  })

  describe('pricing display', () => {
    it('should show pricing information when showPricing is true', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
          showPricing={true}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show pricing for each model (multiple elements with these titles)
      expect(screen.getAllByTitle('Input price').length).toBeGreaterThan(0)
      expect(screen.getAllByTitle('Output price').length).toBeGreaterThan(0)
    })

    it('should not show pricing when showPricing is false', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
          showPricing={false}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should not show pricing
      expect(screen.queryByTitle('Input price')).not.toBeInTheDocument()
      expect(screen.queryByTitle('Output price')).not.toBeInTheDocument()
    })
  })

  describe('capability badges', () => {
    it('should show capability badges when showCapabilities is true', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
          showCapabilities={true}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show capability badges
      expect(screen.getAllByTitle('Vision').length).toBeGreaterThan(0)
      expect(screen.getAllByTitle('Tool Use').length).toBeGreaterThan(0)
      expect(screen.getAllByTitle('Streaming').length).toBeGreaterThan(0)
    })

    it('should not show capability badges when showCapabilities is false', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
          showCapabilities={false}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should not show capability badges
      expect(screen.queryByTitle('Vision')).not.toBeInTheDocument()
      expect(screen.queryByTitle('Tool Use')).not.toBeInTheDocument()
      expect(screen.queryByTitle('Streaming')).not.toBeInTheDocument()
    })
  })

  describe('context window display', () => {
    it('should format context window correctly', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Check context window formatting
      // 200000 -> 200K, 128000 -> 128K, 2097152 -> 2.1M
      expect(screen.getAllByText('200K').length).toBeGreaterThan(0) // Anthropic models
      expect(screen.getAllByText('128K').length).toBeGreaterThan(0) // OpenAI models
      expect(screen.getByText('2.1M')).toBeInTheDocument() // Gemini 1.5 Pro
    })
  })

  describe('accessibility', () => {
    it('should have proper ARIA attributes on trigger', () => {
      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      expect(trigger).toHaveAttribute('aria-haspopup', 'listbox')
      expect(trigger).toHaveAttribute('aria-expanded', 'false')
    })

    it('should update aria-expanded when dropdown is open', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      expect(trigger).toHaveAttribute('aria-expanded', 'true')
    })

    it('should have proper role on model items', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      const options = screen.getAllByRole('option')
      expect(options.length).toBeGreaterThan(0)
    })

    it('should support keyboard navigation for model selection', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Find a model option and focus it
      const modelOption = screen.getByText('Claude 3.5 Sonnet').closest('[role="option"]')
      expect(modelOption).toBeInTheDocument()

      // Simulate Enter key on the option
      if (modelOption) {
        fireEvent.keyDown(modelOption, { key: 'Enter' })
      }

      expect(onModelSelect).toHaveBeenCalledWith('anthropic', 'claude-3-5-sonnet-20241022')
    })

    it('should support Space key for model selection', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      // Open dropdown
      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Find a model option
      const modelOption = screen.getByText('GPT-4o').closest('[role="option"]')
      expect(modelOption).toBeInTheDocument()

      // Simulate Space key on the option
      if (modelOption) {
        fireEvent.keyDown(modelOption, { key: ' ' })
      }

      expect(onModelSelect).toHaveBeenCalledWith('openai', 'gpt-4o')
    })

    it('should have aria-disabled on unavailable model items', async () => {
      const user = userEvent.setup()

      const providersWithUnavailable: ProviderInfo[] = [
        {
          id: 'anthropic',
          name: 'Anthropic',
          models: mockModels.anthropic,
          isAvailable: false,
          statusMessage: 'Unavailable',
        },
      ]

      render(
        <ModelPicker
          providers={providersWithUnavailable}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      const options = screen.getAllByRole('option')
      options.forEach((option) => {
        expect(option).toHaveAttribute('aria-disabled', 'true')
      })
    })
  })

  describe('edge cases', () => {
    it('should handle empty providers array gracefully', () => {
      render(
        <ModelPicker
          providers={[]}
          onModelSelect={onModelSelect}
        />
      )

      expect(screen.getByText('Select model...')).toBeInTheDocument()
    })

    it('should show empty state when opening dropdown with no providers', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={[]}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // The dropdown should be open but empty (no providers to show)
      // The list container should exist
      const list = document.querySelector('.model-picker__list')
      expect(list).toBeInTheDocument()
    })

    it('should filter models by model ID in addition to name', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Search by model ID instead of name
      const searchInput = screen.getByPlaceholderText('Search models...')
      await user.type(searchInput, 'gpt-4o-mini')

      // Should find the model by its ID
      expect(screen.getByText('GPT-4o Mini')).toBeInTheDocument()
      // Should not show other GPT models
      expect(screen.queryByText('Claude 3.5 Sonnet')).not.toBeInTheDocument()
    })

    it('should handle provider with no models', async () => {
      const user = userEvent.setup()

      const providersWithEmpty: ProviderInfo[] = [
        {
          id: 'empty-provider',
          name: 'Empty Provider',
          models: [],
          isAvailable: true,
        },
        ...mockProviders,
      ]

      render(
        <ModelPicker
          providers={providersWithEmpty}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should not show the empty provider group since it has no models
      // But should still show other providers
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()
    })

    it('should toggle dropdown on repeated clicks', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')

      // First click - open
      await user.click(trigger)
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()

      // Second click - close
      await user.click(trigger)
      await waitFor(() => {
        expect(screen.queryByText('Claude 3.5 Sonnet')).not.toBeInTheDocument()
      })

      // Third click - open again
      await user.click(trigger)
      expect(screen.getByText('Claude 3.5 Sonnet')).toBeInTheDocument()
    })

    it('should clear search query when closing dropdown', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')

      // Open and search
      await user.click(trigger)
      const searchInput = screen.getByPlaceholderText('Search models...')
      await user.type(searchInput, 'claude')

      // Close with Escape
      await user.keyboard('{Escape}')

      // Reopen - search should be cleared
      await user.click(trigger)
      const newSearchInput = screen.getByPlaceholderText('Search models...')
      expect(newSearchInput).toHaveValue('')

      // All models should be visible
      expect(screen.getByText('GPT-4o')).toBeInTheDocument()
    })

    it('should not open dropdown when disabled', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
          disabled={true}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Dropdown should not open
      expect(screen.queryByText('Claude 3.5 Sonnet')).not.toBeInTheDocument()
    })

    it('should show default unavailable status when no statusMessage provided', async () => {
      const user = userEvent.setup()

      const providersWithNoMessage: ProviderInfo[] = [
        {
          id: 'disabled-provider',
          name: 'Disabled Provider',
          models: [{
            id: 'test-model',
            name: 'Test Model',
            provider: 'disabled-provider',
            contextWindow: 100000,
            maxOutputTokens: 4096,
            pricePerMInputTokens: 1.0,
            pricePerMOutputTokens: 2.0,
            supportsTools: false,
            supportsStreaming: false,
          }],
          isAvailable: false,
          // No statusMessage provided
        },
      ]

      render(
        <ModelPicker
          providers={providersWithNoMessage}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show default "Unavailable" status
      expect(screen.getByText('Unavailable')).toBeInTheDocument()
    })

    it('should have correct title on trigger with selected model', () => {
      render(
        <ModelPicker
          providers={mockProviders}
          selectedProvider="openai"
          selectedModel="gpt-4o"
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      expect(trigger).toHaveAttribute('title', 'openai / GPT-4o')
    })

    it('should have correct title on trigger without selected model', () => {
      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      expect(trigger).toHaveAttribute('title', 'Select a model')
    })
  })

  describe('pricing formats', () => {
    it('should display free pricing correctly', async () => {
      const user = userEvent.setup()

      const freeProvider: ProviderInfo[] = [
        {
          id: 'free-provider',
          name: 'Free Provider',
          models: [{
            id: 'free-model',
            name: 'Free Model',
            provider: 'free-provider',
            contextWindow: 100000,
            maxOutputTokens: 4096,
            pricePerMInputTokens: 0,
            pricePerMOutputTokens: 0,
            supportsTools: true,
            supportsStreaming: true,
          }],
          isAvailable: true,
        },
      ]

      render(
        <ModelPicker
          providers={freeProvider}
          onModelSelect={onModelSelect}
          showPricing={true}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should show "Free" for zero price
      expect(screen.getAllByText('Free').length).toBeGreaterThanOrEqual(1)
    })

    it('should format very small prices correctly', async () => {
      const user = userEvent.setup()

      const cheapProvider: ProviderInfo[] = [
        {
          id: 'cheap-provider',
          name: 'Cheap Provider',
          models: [{
            id: 'cheap-model',
            name: 'Cheap Model',
            provider: 'cheap-provider',
            contextWindow: 100000,
            maxOutputTokens: 4096,
            pricePerMInputTokens: 0.005,
            pricePerMOutputTokens: 0.008,
            supportsTools: true,
            supportsStreaming: true,
          }],
          isAvailable: true,
        },
      ]

      render(
        <ModelPicker
          providers={cheapProvider}
          onModelSelect={onModelSelect}
          showPricing={true}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Very small prices should show 4 decimal places
      expect(screen.getByText('$0.0050/M')).toBeInTheDocument()
      expect(screen.getByText('$0.0080/M')).toBeInTheDocument()
    })
  })

  describe('provider display names', () => {
    it('should display correct provider names for known providers', async () => {
      const user = userEvent.setup()

      render(
        <ModelPicker
          providers={mockProviders}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      expect(screen.getByText('Anthropic')).toBeInTheDocument()
      expect(screen.getByText('OpenAI')).toBeInTheDocument()
      expect(screen.getByText('Google Gemini')).toBeInTheDocument()
    })

    it('should display provider ID as name for unknown providers', async () => {
      const user = userEvent.setup()

      const unknownProvider: ProviderInfo[] = [
        {
          id: 'custom-llm',
          name: 'Custom LLM',
          models: [{
            id: 'custom-model',
            name: 'Custom Model',
            provider: 'custom-llm',
            contextWindow: 100000,
            maxOutputTokens: 4096,
            pricePerMInputTokens: 1.0,
            pricePerMOutputTokens: 2.0,
            supportsTools: true,
            supportsStreaming: true,
          }],
          isAvailable: true,
        },
      ]

      render(
        <ModelPicker
          providers={unknownProvider}
          onModelSelect={onModelSelect}
        />
      )

      const trigger = screen.getByRole('button')
      await user.click(trigger)

      // Should fall back to using the provider ID
      expect(screen.getByText('custom-llm')).toBeInTheDocument()
    })
  })
})
