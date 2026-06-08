import uiPreset from '../../packages/ui/preset.js'

/** @type {import('tailwindcss').Config} */
export default {
  presets: [uiPreset],
  content: [
    './index.html',
    './src/**/*.{vue,js,ts,jsx,tsx}',
    '../../packages/billing/**/*.{vue,js,ts,jsx,tsx}',
    '../../packages/ui/components/**/*.{vue,js,ts,jsx,tsx}'
  ]
}
