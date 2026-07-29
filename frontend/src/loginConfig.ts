export type LoginPresentation = {
  loginId: string
  password: string
  showDemoCredentials: boolean
}

const demoCredentialsEnabled = import.meta.env.VITE_DEMO_CREDENTIALS !== 'false'

export const loginConfig: LoginPresentation = demoCredentialsEnabled
  ? { loginId: 'admin', password: 'construction-demo', showDemoCredentials: true }
  : { loginId: '', password: '', showDemoCredentials: false }
