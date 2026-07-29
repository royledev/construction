import { createTheme } from '@mui/material/styles'

declare module '@mui/material/styles' {
  interface Palette { blueprint: { main: string; dark: string; pale: string } }
  interface PaletteOptions { blueprint?: { main: string; dark: string; pale: string } }
}

export const theme = createTheme({
  palette: {
    mode: 'light', primary: { main: '#132B3A', contrastText: '#FFFFFF' }, secondary: { main: '#F26B2A', contrastText: '#FFFFFF' },
    background: { default: '#F3F1EC', paper: '#FFFFFF' }, text: { primary: '#27313A', secondary: '#627078' }, success: { main: '#2E7D5B' },
    blueprint: { main: '#2E6F95', dark: '#1D4E6B', pale: '#E4F0F5' },
  },
  typography: { fontFamily: '"Hiragino Sans", "Yu Gothic", "Noto Sans JP", sans-serif', h1: { fontWeight: 800, letterSpacing: '-0.04em' }, h2: { fontWeight: 800, letterSpacing: '-0.025em' }, button: { fontWeight: 700, letterSpacing: '0.03em' } },
  shape: { borderRadius: 8 },
  components: {
    MuiButton: { styleOverrides: { root: { borderRadius: 5, boxShadow: 'none', minHeight: 42 }, containedSecondary: { boxShadow: '0 5px 0 #B84D19', '&:hover': { boxShadow: '0 3px 0 #B84D19', transform: 'translateY(2px)' } } } },
    MuiPaper: { styleOverrides: { root: { boxShadow: '0 2px 8px rgba(19,43,58,0.08)' } } },
    MuiTextField: { defaultProps: { variant: 'outlined', size: 'small' }, styleOverrides: { root: { '& .MuiOutlinedInput-root': { borderRadius: 5, backgroundColor: '#fff' } } } },
  },
})
