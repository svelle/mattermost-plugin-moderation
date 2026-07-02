import React from 'react';

const DEFAULT_VIEWBOX = '0 0 24 24';

// Material Design Icon paths matching the glyphs used in the design.
// Entries may override the viewBox for glyphs from other icon sets.
const PATHS: Record<string, string | {d: string; viewBox: string}> = {
    shield: 'M12,1L3,5V11C3,16.55 6.84,21.74 12,23C17.16,21.74 21,16.55 21,11V5L12,1Z',

    // Material Symbols "shield person" — the plugin's identity icon.
    'shield-person': {
        viewBox: '0 -960 960 960',
        d: 'M485-240Zm26 80H160v-112q0-34 17.5-62.5T224-378q62-31 126-46.5T480-440v80q-56 0-111 13.5T260-306q-9 5-14.5 14t-5.5 20v32h245q4 21 10.5 41t15.5 39Zm209 80q-73-18-116.5-80T560-298v-102l160-80 160 80v102q0 76-43.5 138T720-80Zm0-84q38-18 59-55t21-79v-52l-80-40-80 40v52q0 42 21 79t59 55ZM367-527q-47-47-47-113t47-113q47-47 113-47t113 47q47 47 47 113t-47 113q-47 47-113 47t-113-47Zm169.5-56.5Q560-607 560-640t-23.5-56.5Q513-720 480-720t-56.5 23.5Q400-673 400-640t23.5 56.5Q447-560 480-560t56.5-23.5ZM480-640Zm240 363Z',
    },
    flag: 'M14.4,6L14,4H5V21H7V14H12.6L13,16H20V6H14.4Z',
    clock: 'M12,20A8,8 0 0,1 4,12A8,8 0 0,1 12,4A8,8 0 0,1 20,12A8,8 0 0,1 12,20M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12.5,7H11V13L15.75,15.85L16.5,14.62L12.5,12.25V7Z',
    'volume-off': 'M12,4L9.91,6.09L12,8.18M4.27,3L3,4.27L7.73,9H3V15H7L12,20V13.27L16.25,17.53C15.58,18.04 14.83,18.46 14,18.7V20.77C15.38,20.45 16.63,19.82 17.68,18.96L19.73,21L21,19.73L12,10.73M19,12C19,12.94 18.8,13.82 18.46,14.64L19.97,16.15C20.62,14.91 21,13.5 21,12C21,7.72 18,4.14 14,3.23V5.29C16.89,6.15 19,8.83 19,12M16.5,12C16.5,10.23 15.5,8.71 14,7.97V10.18L16.45,12.63C16.5,12.43 16.5,12.21 16.5,12Z',
    'alert-circle-outline': 'M11,15H13V17H11V15M11,7H13V13H11V7M12,2C6.47,2 2,6.5 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,20A8,8 0 0,1 4,12A8,8 0 0,1 12,4A8,8 0 0,1 20,12A8,8 0 0,1 12,20Z',
    'account-alert-outline': 'M10,4A4,4 0 0,1 14,8A4,4 0 0,1 10,12A4,4 0 0,1 6,8A4,4 0 0,1 10,4M10,6A2,2 0 0,0 8,8A2,2 0 0,0 10,10A2,2 0 0,0 12,8A2,2 0 0,0 10,6M10,13C12.67,13 18,14.33 18,17V20H2V17C2,14.33 7.33,13 10,13M10,14.9C7.03,14.9 3.9,16.36 3.9,17V18.1H16.1V17C16.1,16.36 12.97,14.9 10,14.9M20,12V7H22V13H20M20,17V15H22V17H20Z',
    'account-outline': 'M12,4A4,4 0 0,1 16,8A4,4 0 0,1 12,12A4,4 0 0,1 8,8A4,4 0 0,1 12,4M12,6A2,2 0 0,0 10,8A2,2 0 0,0 12,10A2,2 0 0,0 14,8A2,2 0 0,0 12,6M12,13C14.67,13 20,14.33 20,17V20H4V17C4,14.33 9.33,13 12,13M12,14.9C9.03,14.9 5.9,16.36 5.9,17V18.1H18.1V17C18.1,16.36 14.97,14.9 12,14.9Z',
    'arrow-up': 'M13,20H11V8L5.5,13.5L4.08,12.08L12,4.16L19.92,12.08L18.5,13.5L13,8V20Z',
    'trash-can-outline': 'M9,3V4H4V6H5V19A2,2 0 0,0 7,21H17A2,2 0 0,0 19,19V6H20V4H15V3H9M7,6H17V19H7V6M9,8V17H11V8H9M13,8V17H15V8H13Z',
    cancel: 'M12,2A10,10 0 0,0 2,12A10,10 0 0,0 12,22A10,10 0 0,0 22,12A10,10 0 0,0 12,2M12,4C13.85,4 15.55,4.63 16.9,5.68L5.68,16.9C4.63,15.55 4,13.85 4,12A8,8 0 0,1 12,4M12,20C10.15,20 8.45,19.37 7.1,18.32L18.32,7.1C19.37,8.45 20,10.15 20,12A8,8 0 0,1 12,20Z',
    close: 'M19,6.41L17.59,5L12,10.59L6.41,5L5,6.41L10.59,12L5,17.59L6.41,19L12,13.41L17.59,19L19,17.59L13.41,12L19,6.41Z',
    'check-circle': 'M12,2A10,10 0 0,1 22,12A10,10 0 0,1 12,22A10,10 0 0,1 2,12A10,10 0 0,1 12,2M11,16.5L18,9.5L16.59,8.09L11,13.67L7.91,10.59L6.5,12L11,16.5Z',
    'eye-outline': 'M12,9A3,3 0 0,1 15,12A3,3 0 0,1 12,15A3,3 0 0,1 9,12A3,3 0 0,1 12,9M12,4.5C17,4.5 21.27,7.61 23,12C21.27,16.39 17,19.5 12,19.5C7,19.5 2.73,16.39 1,12C2.73,7.61 7,4.5 12,4.5M3.18,12C4.83,15.36 8.24,17.5 12,17.5C15.76,17.5 19.17,15.36 20.82,12C19.17,8.64 15.76,6.5 12,6.5C8.24,6.5 4.83,8.64 3.18,12Z',
};

// SHIELD_PERSON is exported for surfaces that need the raw glyph (e.g. the
// app bar icon URL, which requires a self-contained image).
export const SHIELD_PERSON = PATHS['shield-person'] as {d: string; viewBox: string};

type Props = {
    name: string;
    size?: number;
    color?: string;
    style?: React.CSSProperties;
};

const Icon = ({name, size = 16, color = 'currentColor', style}: Props) => {
    const entry = PATHS[name] || PATHS.shield;
    const d = typeof entry === 'string' ? entry : entry.d;
    const viewBox = typeof entry === 'string' ? DEFAULT_VIEWBOX : entry.viewBox;
    return (
        <svg
            viewBox={viewBox}
            style={{width: size, height: size, fill: color, flexShrink: 0, ...style}}
            aria-hidden='true'
        >
            <path d={d}/>
        </svg>
    );
};

export default Icon;
