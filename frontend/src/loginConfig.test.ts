import { afterEach, describe, expect, it, vi } from 'vitest'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

describe('loginConfig', () => {
  it('keeps demo defaults available for local development', async () => {
    vi.stubEnv('VITE_DEMO_CREDENTIALS', 'true')
    const { loginConfig } = await import('./loginConfig')
    expect(loginConfig).toEqual({
      loginId: 'admin',
      password: 'construction-demo',
      showDemoCredentials: true,
    })
  })

  it('removes demo credentials from production login presentation', async () => {
    vi.stubEnv('VITE_DEMO_CREDENTIALS', 'false')
    const { loginConfig } = await import('./loginConfig')
    expect(loginConfig).toEqual({
      loginId: '',
      password: '',
      showDemoCredentials: false,
    })
  })
})
