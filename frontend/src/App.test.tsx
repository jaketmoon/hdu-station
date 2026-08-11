import { fireEvent, render, screen } from '@testing-library/react'

import App from './App'

describe('HDU Station shell', () => {
  it('presents one conversation entry without a sandbox mode switch', () => {
    render(<App />)

    expect(screen.getByLabelText('给 HDU Station 发消息')).toBeInTheDocument()
    expect(screen.queryByRole('switch')).not.toBeInTheDocument()
    expect(screen.getByText('需要执行代码时会自动使用安全环境')).toBeInTheDocument()
  })

  it('accepts a suggested course-selection prompt', () => {
    render(<App />)

    fireEvent.click(screen.getByRole('button', { name: '看看这学期能选什么课' }))
    expect(screen.getByLabelText('给 HDU Station 发消息')).toHaveValue(
      '看看这学期能选什么课',
    )
  })
})
