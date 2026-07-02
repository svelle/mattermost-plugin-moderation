import React, {useState} from 'react';
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

type RowProps = {
    icon: string;
    label: string;
    danger?: boolean;
    onClick: () => void;
};

// MenuRow mimics the native profile popover rows: transparent background
// with a hover highlight, regular-weight text, and a dimmed icon. Color is
// reserved for destructive actions, and only on the text and icon.
const MenuRow = ({icon, label, danger, onClick}: RowProps) => {
    const [hover, setHover] = useState(false);
    return (
        <button
            onClick={onClick}
            onMouseEnter={() => setHover(true)}
            onMouseLeave={() => setHover(false)}
            style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                width: '100%',
                height: 32,
                padding: '0 12px',
                border: 'none',
                borderRadius: 4,
                cursor: 'pointer',
                fontFamily: 'inherit',
                fontSize: 14,
                fontWeight: 400,
                textAlign: 'left',
                background: hover ? C.bg3 : 'transparent',
                color: danger ? C.danger : C.fg1,
            }}
        >
            <span style={{display: 'inline-flex', opacity: danger ? 1 : 0.64}}>
                <Icon
                    name={icon}
                    size={16}
                />
            </span>
            <span style={{overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap'}}>{label}</span>
        </button>
    );
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

    return (
        <div style={{marginTop: 8, paddingTop: 8, borderTop: `1px solid ${C.border1}`}}>
            <div style={{padding: '0 12px 4px', fontSize: 10, fontWeight: 600, letterSpacing: 0.6, color: C.fg3}}>
                {'MODERATION'}
            </div>
            <MenuRow
                icon='flag'
                label={`Report ${name}`}
                danger={true}
                onClick={() => run(openReportModal({channelId, targetUserId: user.id, targetName: name}))}
            />
            {status.is_moderator && (
                <>
                    <MenuRow
                        icon='volume-off'
                        label='Mute in channel'
                        onClick={() => run(muteUser(channelId, user.id, name))}
                    />
                    <MenuRow
                        icon='clock'
                        label='Timeout…'
                        onClick={() => run(openTimeoutModal({channelId, targetUserId: user.id, targetName: name}))}
                    />
                    <MenuRow
                        icon='alert-circle-outline'
                        label={`Warn ${name}`}
                        onClick={() => run(warnUser(channelId, user.id, name))}
                    />
                </>
            )}
            {status.is_moderator && !status.is_admin && (
                <MenuRow
                    icon='arrow-up'
                    label='Escalate to admin'
                    onClick={() => run(escalateUser(channelId, user.id))}
                />
            )}
            {status.is_admin && (
                <>
                    <MenuRow
                        icon='close'
                        label='Remove from channel'
                        onClick={() => run(confirmRemove(channelId, user.id, name))}
                    />
                    <MenuRow
                        icon='cancel'
                        label='Ban from server'
                        danger={true}
                        onClick={() => run(confirmBan(channelId, user.id, name))}
                    />
                </>
            )}
        </div>
    );
};

export default PopoverUserActions;
