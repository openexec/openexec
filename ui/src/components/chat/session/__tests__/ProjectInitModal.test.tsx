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

    const submitButton = screen.getByRole('button', { name: /Create Project/i })
    await user.click(submitButton)

    expect(onSubmit).toHaveBeenCalledWith('my-project', '/path/to/project/', 'repository')
  })

  it('requires directory path', () => {
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)

    const submitButton = screen.getByRole('button', { name: /Create Project/i })
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

    const submitButton = screen.getByRole('button', { name: /Create Project/i })
    await user.click(submitButton)

    // "My Project! @2024" -> "my-project--2024" -> "my-project-2024"
    expect(onSubmit).toHaveBeenCalledWith('my-project-2024', '/path', 'repository')
  })

  it('creates a chat project from a name alone', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)

    await user.click(screen.getByRole('radio', { name: /Chat only/i }))
    const submitButton = screen.getByRole('button', { name: /Create Project/i })
    // Neither name nor path yet: nothing to create
    expect(submitButton).toBeDisabled()
    // Chat projects have no repository to pick
    expect(screen.getByLabelText(/Directory Path/i)).not.toBeRequired()

    await user.type(screen.getByLabelText(/Project Name/i), 'Trip Ideas')
    await user.click(submitButton)

    expect(onSubmit).toHaveBeenCalledWith('trip-ideas', '', 'chat')
  })

  it('keeps the directory requirement for repository projects', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<ProjectInitModal onSubmit={onSubmit} onCancel={() => {}} apiUrl="" />)

    await user.type(screen.getByLabelText(/Project Name/i), 'app')
    expect(screen.getByRole('radio', { name: /^Repository/i })).toBeChecked()
    expect(screen.getByRole('button', { name: /Create Project/i })).toBeDisabled()
    expect(onSubmit).not.toHaveBeenCalled()
  })
})
