import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ChatInput from '../ChatInput'

describe('ChatInput', () => {
  it('trims whitespace and submits', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)
    
    const textarea = screen.getByPlaceholderText(/Type your message/i)
    fireEvent.change(textarea, { target: { value: '  hello world  ' } })
    
    const sendButton = screen.getByRole('button')
    fireEvent.click(sendButton)
    
    expect(onSubmit).toHaveBeenCalledWith('hello world')
  })

  it('does not submit empty content', () => {
    const onSubmit = vi.fn()
    render(<ChatInput onSubmit={onSubmit} />)
    
    const textarea = screen.getByPlaceholderText(/Type your message/i)
    fireEvent.change(textarea, { target: { value: '   ' } })
    
    const sendButton = screen.getByRole('button')
    fireEvent.click(sendButton)
    
    expect(onSubmit).not.toHaveBeenCalled()
  })
})
