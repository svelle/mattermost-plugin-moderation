import {useEffect} from 'react';

// useEscape closes an overlay when the user presses Escape.
export const useEscape = (onClose: () => void, enabled = true) => {
    useEffect(() => {
        if (!enabled) {
            return undefined;
        }
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                onClose();
            }
        };
        document.addEventListener('keydown', onKeyDown);
        return () => document.removeEventListener('keydown', onKeyDown);
    }, [onClose, enabled]);
};
