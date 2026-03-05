import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProjectInitModal from '../ProjectInitModal'

describe('ProjectInitModal', () => {
  it('submits trimmed name and path', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)

    const nameInput = screen.getByLabelText(/Project Name/i)
    const pathInput = screen.getByLabelText(/Directory Path/i)

    await user.type(nameInput, '  my-project  ')
    await user.type(pathInput, '  /path/to/project/  ')

    const submitButton = screen.getByRole('button', { name: /Initialize/i })
    await user.click(submitButton)

    expect(onSubmit).toHaveBeenCalledWith('my-project', '/path/to/project/')
  })

  it('requires directory path', () => {
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)
    
    const submitButton = screen.getByRole('button', { name: /Initialize/i })
    expect(submitButton).toBeDisabled()
  })

  it('sanitizes project name (lowercase and illegal chars)', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)

    const nameInput = screen.getByLabelText(/Project Name/i)
    const pathInput = screen.getByLabelText(/Directory Path/i)

    await user.type(nameInput, 'My Project! @2024')
    await user.type(pathInput, '/path')

    const submitButton = screen.getByRole('button', { name: /Initialize/i })
    await user.click(submitButton)

    // "My Project! @2024" -> "my-project--2024" -> "my-project-2024"
    expect(onSubmit).toHaveBeenCalledWith('my-project-2024', '/path')
  })
})
