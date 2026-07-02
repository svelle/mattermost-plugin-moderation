import React, {useEffect} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import {fetchStatus} from '../actions';
import {getCurrentChannelId, getStatusForCurrentChannel} from '../selectors';

// ChannelHeaderButton is the shield icon in the channel header. It also
// keeps the per-channel moderation status fresh: it is always mounted, so it
// refetches whenever the user switches channels.
const ChannelHeaderButton = () => {
    const dispatch = useDispatch();
    const channelId = useSelector((state: GlobalState) => getCurrentChannelId(state));
    const status = useSelector((state: GlobalState) => getStatusForCurrentChannel(state));

    useEffect(() => {
        // @ts-expect-error thunk actions are supported by the webapp store
        dispatch(fetchStatus(channelId));
    }, [channelId, dispatch]);

    return (
        <span style={{position: 'relative', display: 'inline-flex', alignItems: 'center', justifyContent: 'center'}}>
            <i
                className='icon icon-shield-outline'
                style={{fontSize: 18, lineHeight: 1}}
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
