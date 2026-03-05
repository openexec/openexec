import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ChatInput from '../ChatInput'

describe('ChatInput', () => {
  it('trims whitespace and submits', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)

    const textarea = screen.getByPlaceholderText(/Type your message/i)
    await user.type(textarea, '  hello world  ')

    const sendButton = screen.getByRole('button')
    await user.click(sendButton)

    expect(onSubmit).toHaveBeenCalledWith('hello world')
  })

  it('does not submit empty content', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)

    const textarea = screen.getByPlaceholderText(/Type your message/i)
    await user.clear(textarea)
    await user.type(textarea, '   ')

    const sendButton = screen.getByRole('button')
    await user.click(sendButton)

    expect(onSubmit).not.toHaveBeenCalled()
  })
})
