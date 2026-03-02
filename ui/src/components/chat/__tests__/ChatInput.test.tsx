/**
 * ChatInput Component Tests
 *
 * Tests for the message input area.
 *
 * @module components/chat/__tests__/ChatInput.test
 */

import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ChatInput from '../input/ChatInput'

// Mock child components
vi.mock('../input/InputTextarea', () => ({
  default: ({
    value,
    onChange,
    onSubmit,
    placeholder,
    disabled,
  }: {
    value: string
    onChange: (value: string) => void
    onSubmit: () => void
    placeholder?: string
    disabled?: boolean
  }) => (
    <textarea
      data-testid="input-textarea"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault()
          onSubmit()
        }
      }}
      placeholder={placeholder}
      disabled={disabled}
    />
  ),
}))

vi.mock('../input/SendButton', () => ({
  default: ({
    onClick,
    disabled,
    isLoading,
  }: {
    onClick: () => void
    disabled?: boolean
    isLoading?: boolean
  }) => (
    <button
      data-testid="send-button"
      onClick={onClick}
      disabled={disabled}
      data-loading={isLoading}
    >
      Send
    </button>
  ),
}))

vi.mock('../input/InputToolbar', () => ({
  default: () => <div data-testid="input-toolbar" />,
}))

describe('ChatInput', () => {
  it('renders textarea with placeholder', () => {
    render(<ChatInput placeholder="Type here..." />)

    const textarea = screen.getByTestId('input-textarea')
    expect(textarea).toHaveAttribute('placeholder', 'Type here...')
  })

  it('renders with default placeholder when not provided', () => {
    render(<ChatInput />)

    const textarea = screen.getByTestId('input-textarea')
    expect(textarea).toHaveAttribute(
      'placeholder',
      'Type your message... (Shift+Enter for new line)'
    )
  })

  it('disables textarea when disabled prop is true', () => {
    render(<ChatInput disabled={true} />)

    const textarea = screen.getByTestId('input-textarea')
    expect(textarea).toBeDisabled()
  })

  it('disables textarea when isSubmitting is true', () => {
    render(<ChatInput isSubmitting={true} />)

    const textarea = screen.getByTestId('input-textarea')
    expect(textarea).toBeDisabled()
  })

  it('updates content when typing', () => {
    render(<ChatInput />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: 'Hello' } })

    expect(textarea).toHaveValue('Hello')
  })

  it('disables send button when content is empty', () => {
    render(<ChatInput />)

    const button = screen.getByTestId('send-button')
    expect(button).toBeDisabled()
  })

  it('enables send button when content is not empty', () => {
    render(<ChatInput />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: 'Hello' } })

    const button = screen.getByTestId('send-button')
    expect(button).not.toBeDisabled()
  })

  it('calls onSubmit with content when send button is clicked', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: 'Hello World' } })

    const button = screen.getByTestId('send-button')
    fireEvent.click(button)

    expect(onSubmit).toHaveBeenCalledWith('Hello World')
  })

  it('clears content after successful submit', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: 'Hello' } })
    fireEvent.click(screen.getByTestId('send-button'))

    expect(textarea).toHaveValue('')
  })

  it('calls onSubmit when Enter is pressed', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: 'Hello' } })
    fireEvent.keyDown(textarea, { key: 'Enter', shiftKey: false })

    expect(onSubmit).toHaveBeenCalledWith('Hello')
  })

  it('does not call onSubmit when content is only whitespace', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: '   ' } })
    fireEvent.click(screen.getByTestId('send-button'))

    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('does not call onSubmit when disabled', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} disabled={true} />)

    const textarea = screen.getByTestId('input-textarea')
    fireEvent.change(textarea, { target: { value: 'Hello' } })

    // Try to submit via Enter (even though textarea is disabled)
    fireEvent.keyDown(textarea, { key: 'Enter', shiftKey: false })

    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('renders help text', () => {
    render(<ChatInput />)

    expect(screen.getByText(/Press/)).toBeInTheDocument()
    expect(screen.getByText('Enter')).toBeInTheDocument()
    expect(screen.getByText('Shift+Enter')).toBeInTheDocument()
  })

  it('renders input toolbar', () => {
    render(<ChatInput />)

    expect(screen.getByTestId('input-toolbar')).toBeInTheDocument()
  })

  it('shows loading state on send button when isSubmitting', () => {
    render(<ChatInput isSubmitting={true} />)

    const button = screen.getByTestId('send-button')
    expect(button).toHaveAttribute('data-loading', 'true')
  })
})
