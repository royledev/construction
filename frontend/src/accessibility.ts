export function editableCellA11y(label: string) { return { role: 'button' as const, tabIndex: 0, 'aria-label': `${label}を編集` } }
export function isEditActivation(key: string) { return key === 'Enter' || key === ' ' }
