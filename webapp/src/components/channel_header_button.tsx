import React from 'react';
import {useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import Icon from './icons';

import {getStatusForCurrentChannel} from '../selectors';

// ChannelHeaderButton is the shield icon in the channel header, with a badge
// for open reports. Purely presentational — the webapp doesn't render it at
// all when the app bar is enabled, so status fetching lives in
// ModerationRoot, which is always mounted.
const ChannelHeaderButton = () => {
    const status = useSelector((state: GlobalState) => getStatusForCurrentChannel(state));

    return (
        <span style={{position: 'relative', display: 'inline-flex', alignItems: 'center', justifyContent: 'center'}}>
            {/* currentColor keeps the icon matched to the user's theme in
                both the header button and the app bar. */}
            <Icon
                name='shield-person'
                size={18}
            />
            {status.is_moderator && status.open_report_count > 0 && (
                <span
                    style={{
                        position: 'absolute',
                        top: -7,
                        right: -10,
                        minWidth: 16,
                        height: 16,
                        padding: '0 4px',
                        borderRadius: 8,
                        background: 'var(--dnd-indicator)',
                        color: '#fff',
                        fontSize: 10,
                        fontWeight: 700,
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                    }}
                >
                    {status.open_report_count}
                </span>
            )}
        </span>
    );
};

export default ChannelHeaderButton;
