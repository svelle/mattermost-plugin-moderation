import React from 'react';
import {useDispatch, useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';
import type {UserProfile} from '@mattermost/types/users';

import Icon from './icons';
import {C} from './styles';

import {confirmBan, confirmRemove, escalateUser, muteUser, openReportModal, openTimeoutModal, warnUser} from '../actions';
import {getCurrentChannelId, getCurrentUserId, getStatusForCurrentChannel} from '../selectors';

type Props = {
    user: UserProfile;
    hide?: () => void;
};

const displayNameOf = (user: UserProfile): string => {
    if (user.nickname) {
        return user.nickname;
    }
    const fullName = [user.first_name, user.last_name].filter(Boolean).join(' ');
    return fullName || `@${user.username}`;
};

// PopoverUserActions adds the moderation actions from the design's profile
// popover: report for everyone, mute/timeout/warn/escalate for moderators,
// and remove/ban for admins.
const PopoverUserActions = ({user, hide}: Props) => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => void;
    const channelId = useSelector((state: GlobalState) => getCurrentChannelId(state));
    const currentUserId = useSelector((state: GlobalState) => getCurrentUserId(state));
    const status = useSelector((state: GlobalState) => getStatusForCurrentChannel(state));

    if (!user || user.id === currentUserId || user.is_bot || !channelId) {
        return null;
    }

    const name = displayNameOf(user);
    const run = (action: unknown) => {
        hide?.();
        dispatchThunk(action);
    };

    const buttonStyle = (danger?: boolean): React.CSSProperties => ({
        display: 'flex',
        alignItems: 'center',
        gap: 9,
        width: '100%',
        height: 34,
        padding: '0 12px',
        borderRadius: 5,
        cursor: 'pointer',
        fontFamily: 'inherit',
        fontSize: 13,
        fontWeight: 600,
        background: danger ? C.redTint : C.bg3,
        color: danger ? C.danger : C.fg1,
        border: 'none',
        marginTop: 6,
    });

    return (
        <div style={{padding: '4px 0'}}>
            <button
                style={buttonStyle(true)}
                onClick={() => run(openReportModal({channelId, targetUserId: user.id, targetName: name}))}
            >
                <Icon
                    name='flag'
                    size={16}
                />
                <span>{'Report '}{name}</span>
            </button>
            {status.is_moderator && (
                <>
                    <button
                        style={buttonStyle()}
                        onClick={() => run(muteUser(channelId, user.id, name))}
                    >
                        <Icon
                            name='volume-off'
                            size={16}
                        />
                        <span>{'Mute in channel'}</span>
                    </button>
                    <button
                        style={buttonStyle()}
                        onClick={() => run(openTimeoutModal({channelId, targetUserId: user.id, targetName: name}))}
                    >
                        <Icon
                            name='clock'
                            size={16}
                        />
                        <span>{'Timeout…'}</span>
                    </button>
                    <button
                        style={buttonStyle()}
                        onClick={() => run(warnUser(channelId, user.id, name))}
                    >
                        <Icon
                            name='alert-circle-outline'
                            size={16}
                        />
                        <span>{'Warn '}{name}</span>
                    </button>
                </>
            )}
            {status.is_moderator && !status.is_admin && (
                <button
                    style={buttonStyle()}
                    onClick={() => run(escalateUser(channelId, user.id))}
                >
                    <Icon
                        name='arrow-up'
                        size={16}
                    />
                    <span>{'Escalate to admin'}</span>
                </button>
            )}
            {status.is_admin && (
                <>
                    <button
                        style={buttonStyle()}
                        onClick={() => run(confirmRemove(channelId, user.id, name))}
                    >
                        <Icon
                            name='close'
                            size={16}
                        />
                        <span>{'Remove from channel'}</span>
                    </button>
                    <button
                        style={buttonStyle(true)}
                        onClick={() => run(confirmBan(channelId, user.id, name))}
                    >
                        <Icon
                            name='cancel'
                            size={16}
                        />
                        <span>{'Ban from server'}</span>
                    </button>
                </>
            )}
        </div>
    );
};

export default PopoverUserActions;
