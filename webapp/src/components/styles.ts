// Theme-aware tokens mapping the design's Compass variables onto the
// Mattermost webapp theme, plus the Compass palette hexes used for status
// colors in the mockup.
export const C = {
    fg1: 'var(--center-channel-color)',
    fg2: 'rgba(var(--center-channel-color-rgb),0.72)',
    fg3: 'rgba(var(--center-channel-color-rgb),0.56)',
    border1: 'rgba(var(--center-channel-color-rgb),0.08)',
    border2: 'rgba(var(--center-channel-color-rgb),0.16)',
    border3: 'rgba(var(--center-channel-color-rgb),0.24)',
    bg2: 'rgba(var(--center-channel-color-rgb),0.04)',
    bg3: 'rgba(var(--center-channel-color-rgb),0.08)',
    centerBg: 'var(--center-channel-bg)',
    buttonBg: 'var(--button-bg)',
    buttonTint: 'rgba(var(--button-bg-rgb),0.1)',
    buttonTintFaint: 'rgba(var(--button-bg-rgb),0.05)',
    danger: 'var(--error-text)',

    blue500: '#1c58d9',
    blue600: '#174ab5',
    green500: '#3db887',
    green600: '#339970',
    green700: '#297a5a',
    orange500: '#e07315',
    orange700: '#954d0e',
    purple500: '#484fad',
    purple600: '#3c4290',
    red500: '#c43133',
    red600: '#a3292b',
    yellow600: '#cc8f00',
    yellow700: '#a37200',
    neutral500: '#8d93a5',

    redTint: 'rgba(196,49,51,0.08)',
    redTintStrong: 'rgba(196,49,51,0.12)',
    blueTint: 'rgba(28,88,217,0.12)',
    orangeTint: 'rgba(224,115,21,0.14)',
    purpleTint: 'rgba(72,79,173,0.14)',
    greenTint: 'rgba(51,153,112,0.14)',
    yellowTint: 'rgba(204,143,0,0.16)',
};

export const font = 'inherit';

// timeAgo renders a compact relative timestamp like the mockup's "2m ago".
export const timeAgo = (millis: number): string => {
    const diff = Date.now() - millis;
    const minutes = Math.floor(diff / 60000);
    if (minutes < 1) {
        return 'just now';
    }
    if (minutes < 60) {
        return `${minutes}m ago`;
    }
    const hours = Math.floor(minutes / 60);
    if (hours < 24) {
        return `${hours}h ago`;
    }
    return `${Math.floor(hours / 24)}d ago`;
};
