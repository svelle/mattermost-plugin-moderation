import React, {useEffect} from 'react';
import {useDispatch, useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import MembersTab from './members_tab';
import ReportCard from './report_card';

import {fetchMembers, fetchReports, setRHSTab} from '../../actions';
import {getCurrentChannelId, getCurrentUserId, getStatusForCurrentChannel, pluginState} from '../../selectors';
import Icon from '../icons';
import {C} from '../styles';

// RHSPanel is the "Moderation" right-hand sidebar: the report queue for
// moderators, plus a members & roles tab for admins.
const RHSPanel = () => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => void;
    const channelId = useSelector((state: GlobalState) => getCurrentChannelId(state));
    const currentUserId = useSelector((state: GlobalState) => getCurrentUserId(state));
    const status = useSelector((state: GlobalState) => getStatusForCurrentChannel(state));
    const reports = useSelector((state: GlobalState) => pluginState(state)?.reports[channelId]);
    let tab = useSelector((state: GlobalState) => pluginState(state)?.rhsTab || 'reports');
    if (tab === 'members' && !status.is_admin) {
        tab = 'reports';
    }

    useEffect(() => {
        if (status.is_moderator) {
            dispatchThunk(fetchReports(channelId));
        }
    }, [channelId, status.is_moderator]);

    useEffect(() => {
        if (tab === 'members' && status.is_admin) {
            dispatchThunk(fetchMembers(channelId));
        }
    }, [channelId, tab, status.is_admin]);

    if (!status.is_moderator) {
        return (
            <div style={{padding: '32px 24px', textAlign: 'center', color: C.fg3, fontSize: 13, lineHeight: 1.5}}>
                <Icon
                    name='shield'
                    size={32}
                    color={C.fg3}
                    style={{marginBottom: 10}}
                />
                <div style={{fontWeight: 600, color: C.fg2, fontSize: 14, marginBottom: 4}}>{'Moderation tools'}</div>
                {'You don\'t have moderation access in this channel. You can still report messages and people from the message menu.'}
            </div>
        );
    }

    const openCount = (reports || []).filter((r) => r.status !== 'resolved').length;

    const tabs: Array<{id: 'reports' | 'members'; label: string; count?: number}> = [
        {id: 'reports', label: 'Reports', count: openCount},
    ];
    if (status.is_admin) {
        tabs.push({id: 'members', label: 'Members & roles'});
    }

    return (
        <div style={{display: 'flex', flexDirection: 'column', height: '100%', background: C.centerBg, color: C.fg1}}>
            <div style={{display: 'flex', gap: 2, padding: '8px 12px 0', borderBottom: `1px solid ${C.border1}`, flexShrink: 0}}>
                {tabs.map((t) => (
                    <button
                        key={t.id}
                        onClick={() => dispatch(setRHSTab(t.id))}
                        style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: 6,
                            height: 36,
                            padding: '0 12px',
                            border: 'none',
                            background: 'transparent',
                            cursor: 'pointer',
                            fontFamily: 'inherit',
                            fontSize: 13,
                            fontWeight: 600,
                            color: tab === t.id ? C.fg1 : C.fg3,
                            borderBottom: `2px solid ${tab === t.id ? C.buttonBg : 'transparent'}`,
                            marginBottom: -1,
                        }}
                    >
                        <span>{t.label}</span>
                        {Boolean(t.count) && (
                            <span style={{minWidth: 18, height: 18, padding: '0 5px', borderRadius: 9, background: 'var(--dnd-indicator)', color: '#fff', fontSize: 11, fontWeight: 700, display: 'inline-flex', alignItems: 'center', justifyContent: 'center'}}>
                                {t.count}
                            </span>
                        )}
                    </button>
                ))}
            </div>

            {tab === 'reports' && (
                <div style={{flex: 1, overflowY: 'auto', padding: '14px 12px', display: 'flex', flexDirection: 'column', gap: 10}}>
                    {(reports || []).length === 0 && (
                        <div style={{padding: '28px 16px', textAlign: 'center', color: C.fg3, fontSize: 13, lineHeight: 1.5}}>
                            <Icon
                                name='check-circle'
                                size={28}
                                color={C.green500}
                                style={{marginBottom: 8}}
                            />
                            <div style={{fontWeight: 600, color: C.fg2, fontSize: 14, marginBottom: 3}}>{'No reports'}</div>
                            {'Reports from members of this channel will show up here.'}
                        </div>
                    )}
                    {(reports || []).map((report) => (
                        <ReportCard
                            key={report.id}
                            report={report}
                            isAdmin={status.is_admin}
                        />
                    ))}
                </div>
            )}

            {tab === 'members' && (
                <MembersTab
                    channelId={channelId}
                    currentUserId={currentUserId}
                />
            )}
        </div>
    );
};

export default RHSPanel;
