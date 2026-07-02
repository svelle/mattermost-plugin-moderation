import React, {useState} from 'react';
import {useDispatch} from 'react-redux';

import {confirmBan, confirmRemove, liftRestrictions, openTimeoutModal, setModerator, unbanUser} from '../../actions';
import type {MemberInfo} from '../../types';
import Avatar from '../avatar';
import {useEscape} from '../hooks';
import Icon from '../icons';
import {C} from '../styles';

type Props = {
    member: MemberInfo;
    channelId: string;
    currentUserId: string;
};

type MenuItem = {
    label: string;
    icon: string;
    color?: string;
    onClick: () => void;
};

const MemberRow = ({member, channelId, currentUserId}: Props) => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => void;
    const [menuOpen, setMenuOpen] = useState(false);
    useEscape(() => setMenuOpen(false), menuOpen);

    let tag = {label: 'Member', bg: C.bg3, color: C.fg2};
    if (member.is_banned) {
        tag = {label: 'Banned', bg: C.redTintStrong, color: C.red600};
    } else if (member.is_admin) {
        tag = {label: 'Admin', bg: C.purpleTint, color: C.purple600};
    } else if (member.is_moderator) {
        tag = {label: 'Moderator', bg: C.blueTint, color: C.blue600};
    }

    const isSelf = member.user_id === currentUserId;
    const restricted = member.muted || Boolean(member.timeout_until);

    const items: MenuItem[] = [];
    if (!isSelf && !member.is_admin) {
        items.push({
            label: member.is_moderator ? 'Remove moderator role' : 'Make moderator',
            icon: member.is_moderator ? 'account-outline' : 'shield',
            onClick: () => dispatchThunk(setModerator(channelId, member.user_id, member.display_name, !member.is_moderator)),
        });
        items.push({
            label: 'Timeout…',
            icon: 'clock',
            onClick: () => dispatch(openTimeoutModal({channelId, targetUserId: member.user_id, targetName: member.display_name})),
        });
        if (restricted) {
            items.push({
                label: member.muted ? 'Lift mute' : 'Lift timeout',
                icon: 'check-circle',
                color: C.green700,
                onClick: () => dispatchThunk(liftRestrictions(channelId, member.user_id, member.display_name)),
            });
        }
        items.push({
            label: 'Remove from channel',
            icon: 'close',
            onClick: () => dispatch(confirmRemove(channelId, member.user_id, member.display_name)),
        });
        if (member.is_banned) {
            items.push({
                label: 'Unban member',
                icon: 'shield',
                color: C.green700,
                onClick: () => dispatchThunk(unbanUser(channelId, member.user_id, member.display_name)),
            });
        } else {
            items.push({
                label: 'Ban from server',
                icon: 'cancel',
                color: C.danger,
                onClick: () => dispatch(confirmBan(channelId, member.user_id, member.display_name)),
            });
        }
    }

    return (
        <div style={{display: 'flex', alignItems: 'center', gap: 10, padding: '9px 6px', borderBottom: `1px solid ${C.border1}`, position: 'relative'}}>
            <Avatar userId={member.user_id}/>
            <div style={{flex: 1, minWidth: 0}}>
                <div style={{display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap'}}>
                    <span style={{fontSize: 14, fontWeight: 600, color: C.fg1}}>{member.display_name}</span>
                    <span style={{display: 'inline-flex', alignItems: 'center', gap: 3, height: 17, padding: '0 6px', borderRadius: 9, background: tag.bg, color: tag.color, fontSize: 10, fontWeight: 700, letterSpacing: 0.3}}>
                        {tag.label}
                    </span>
                    {member.muted && !member.is_banned && (
                        <span style={{display: 'inline-flex', alignItems: 'center', gap: 3, height: 17, padding: '0 6px', borderRadius: 9, background: C.bg3, color: C.fg2, fontSize: 10, fontWeight: 700, letterSpacing: 0.3}}>
                            <Icon
                                name='volume-off'
                                size={11}
                            />
                            {'MUTED'}
                        </span>
                    )}
                    {Boolean(member.timeout_until) && !member.is_banned && (
                        <span style={{display: 'inline-flex', alignItems: 'center', gap: 3, height: 17, padding: '0 6px', borderRadius: 9, background: C.yellowTint, color: C.yellow700, fontSize: 10, fontWeight: 700, letterSpacing: 0.3}}>
                            <Icon
                                name='clock'
                                size={11}
                            />
                            {'TIMED OUT'}{member.timeout_label ? ` · ${member.timeout_label}` : ''}
                        </span>
                    )}
                </div>
                <div style={{fontSize: 12, color: C.fg3}}>{'@'}{member.username}</div>
            </div>
            {items.length > 0 && (
                <button
                    onClick={() => setMenuOpen(!menuOpen)}
                    style={{width: 30, height: 30, border: 'none', background: menuOpen ? C.bg3 : 'transparent', borderRadius: 5, cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center', color: C.fg2, fontSize: 17}}
                    aria-label={`Member actions for ${member.display_name}`}
                    aria-haspopup='menu'
                    aria-expanded={menuOpen}
                >
                    <i className='icon icon-dots-vertical'/>
                </button>
            )}
            {menuOpen && (
                <>
                    <div
                        onClick={() => setMenuOpen(false)}
                        style={{position: 'fixed', inset: 0, zIndex: 10}}
                    />
                    <div
                        role='menu'
                        aria-label={`Member actions for ${member.display_name}`}
                        style={{position: 'absolute', right: 6, top: 44, width: 210, background: C.centerBg, border: `1px solid ${C.border2}`, borderRadius: 8, boxShadow: '0 6px 14px rgba(0,0,0,0.12)', padding: 6, zIndex: 20}}
                    >
                        {items.map((item, index) => (
                            <button
                                key={item.label}
                                role='menuitem'
                                autoFocus={index === 0}
                                onClick={() => {
                                    setMenuOpen(false);
                                    item.onClick();
                                }}
                                style={{width: '100%', display: 'flex', alignItems: 'center', gap: 10, height: 34, padding: '0 10px', border: 'none', borderRadius: 5, background: 'transparent', cursor: 'pointer', fontFamily: 'inherit', fontSize: 13, fontWeight: 500, textAlign: 'left', color: item.color || C.fg1}}
                            >
                                <Icon
                                    name={item.icon}
                                    size={16}
                                />
                                <span>{item.label}</span>
                            </button>
                        ))}
                    </div>
                </>
            )}
        </div>
    );
};

export default MemberRow;
