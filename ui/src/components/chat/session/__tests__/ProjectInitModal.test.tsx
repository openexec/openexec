import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ProjectInitModal from '../ProjectInitModal'

describe('ProjectInitModal', () => {
  it('submits trimmed name and path', () => {
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)
    
    const nameInput = screen.getByLabelText(/Project Name/i)
    const pathInput = screen.getByLabelText(/Directory Path/i)
    
    fireEvent.change(nameInput, { target: { value: '  my-project  ' } })
    fireEvent.change(pathInput, { target: { value: '  /path/to/project/  ' } })
    
    const submitButton = screen.getByRole('button', { name: /Initialize/i })
    fireEvent.click(submitButton)
    
    expect(onSubmit).toHaveBeenCalledWith('my-project', '/path/to/project/')
  })

  it('requires directory path', () => {
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)
    
    const submitButton = screen.getByRole('button', { name: /Initialize/i })
    expect(submitButton).toBeDisabled()
  })

  it('sanitizes project name (lowercase and illegal chars)', () => {
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)
    
    const nameInput = screen.getByLabelText(/Project Name/i)
    const pathInput = screen.getByLabelText(/Directory Path/i)
    
    fireEvent.change(nameInput, { target: { value: 'My Project! @2024' } })
    fireEvent.change(pathInput, { target: { value: '/path' } })
    
    const submitButton = screen.getByRole('button', { name: /Initialize/i })
    fireEvent.click(submitButton)
    
    // "My Project! @2024" -> "my-project--2024" -> "my-project-2024"
    expect(onSubmit).toHaveBeenCalledWith('my-project-2024', '/path')
  })
})
