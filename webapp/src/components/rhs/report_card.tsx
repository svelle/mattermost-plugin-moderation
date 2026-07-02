import React from 'react';
import {useDispatch} from 'react-redux';

import {confirmBan, dismissReport, escalateUser, muteUser, openTimeoutModal} from '../../actions';
import type {Report} from '../../types';
import Avatar from '../avatar';
import Icon from '../icons';
import {C, timeAgo} from '../styles';

const statusStyle: Record<string, {label: string; color: string; bg: string; accent: string}> = {
    open: {label: 'Needs review', color: C.orange700, bg: C.orangeTint, accent: C.orange500},
    escalated: {label: 'Escalated', color: C.purple600, bg: C.purpleTint, accent: C.purple500},
    resolved: {label: 'Resolved', color: C.green700, bg: C.greenTint, accent: C.green500},
};

const actionButtonColors: Record<'primary' | 'danger' | 'ghost', {background: string; color: string; border: string}> = {
    primary: {background: C.buttonTint, color: C.buttonBg, border: 'none'},
    danger: {background: C.redTint, color: C.danger, border: 'none'},
    ghost: {background: 'transparent', color: C.fg2, border: `1px solid ${C.border2}`},
};

const actionButtonStyle = (kind: 'primary' | 'danger' | 'ghost'): React.CSSProperties => ({
    display: 'inline-flex',
    alignItems: 'center',
    gap: 5,
    height: 30,
    padding: '0 11px',
    borderRadius: 5,
    cursor: 'pointer',
    fontFamily: 'inherit',
    fontSize: 12,
    fontWeight: 600,
    ...actionButtonColors[kind],
});

type Props = {
    report: Report;
    isAdmin: boolean;
};

const ReportCard = ({report, isAdmin}: Props) => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => void;
    const status = statusStyle[report.status] || statusStyle.open;
    const active = report.status !== 'resolved';

    return (
        <div
            style={{
                border: `1px solid ${C.border1}`,
                borderLeft: `3px solid ${status.accent}`,
                borderRadius: 8,
                padding: '12px 13px',
                background: C.centerBg,
                boxShadow: '0 1px 2px rgba(0,0,0,0.08)',
            }}
        >
            <div style={{display: 'flex', alignItems: 'center', gap: 8, marginBottom: 9}}>
                <span style={{display: 'inline-flex', alignItems: 'center', height: 20, padding: '0 8px', borderRadius: 10, background: status.bg, color: status.color, fontSize: 11, fontWeight: 700}}>
                    {status.label}
                </span>
                <span style={{display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 12, color: C.fg2, fontWeight: 600}}>
                    <Icon
                        name='flag'
                        size={13}
                        color={C.fg3}
                    />
                    {report.reason}
                </span>
                <div style={{flex: 1}}/>
                <span style={{fontSize: 11, color: C.fg3}}>{timeAgo(report.create_at)}</span>
            </div>

            <div style={{display: 'flex', alignItems: 'center', gap: 9, marginBottom: 9}}>
                <Avatar userId={report.target_user_id}/>
                <div style={{minWidth: 0}}>
                    <div style={{fontSize: 14, fontWeight: 600, lineHeight: 1.2, color: C.fg1}}>{report.target_display_name}</div>
                    <div style={{fontSize: 12, color: C.fg3}}>{'Reported by '}{report.reporter_name}</div>
                </div>
            </div>

            {Boolean(report.excerpt) && (
                <div style={{fontSize: 13, lineHeight: '18px', color: C.fg2, background: C.bg2, borderRadius: 6, padding: '9px 11px', marginBottom: 11}}>
                    {report.excerpt}
                </div>
            )}
            {Boolean(report.note) && (
                <div style={{fontSize: 12, lineHeight: '17px', color: C.fg3, fontStyle: 'italic', padding: '0 2px', marginBottom: 11}}>
                    {'Note: '}{report.note}
                </div>
            )}

            {active ? (
                <div style={{display: 'flex', flexWrap: 'wrap', gap: 7}}>
                    <button
                        style={actionButtonStyle('primary')}
                        onClick={() => dispatch(openTimeoutModal({
                            channelId: report.channel_id,
                            targetUserId: report.target_user_id,
                            targetName: report.target_display_name,
                            reportId: report.id,
                        }))}
                    >
                        <Icon
                            name='clock'
                            size={14}
                        />
                        {'Timeout'}
                    </button>
                    <button
                        style={actionButtonStyle('primary')}
                        onClick={() => dispatchThunk(muteUser(report.channel_id, report.target_user_id, report.target_display_name, report.id))}
                    >
                        <Icon
                            name='volume-off'
                            size={14}
                        />
                        {'Mute'}
                    </button>
                    {isAdmin ? (
                        <button
                            style={actionButtonStyle('danger')}
                            onClick={() => dispatch(confirmBan(report.channel_id, report.target_user_id, report.target_display_name, report.id))}
                        >
                            <Icon
                                name='cancel'
                                size={14}
                            />
                            {'Ban'}
                        </button>
                    ) : (
                        <button
                            style={actionButtonStyle('primary')}
                            onClick={() => dispatchThunk(escalateUser(report.channel_id, report.target_user_id))}
                        >
                            <Icon
                                name='arrow-up'
                                size={14}
                            />
                            {'Escalate'}
                        </button>
                    )}
                    <button
                        style={actionButtonStyle('ghost')}
                        onClick={() => dispatchThunk(dismissReport(report.channel_id, report.id))}
                    >
                        <Icon
                            name='close'
                            size={14}
                        />
                        {'Dismiss'}
                    </button>
                </div>
            ) : (
                <div style={{display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 12, color: C.green600, fontWeight: 600}}>
                    <Icon
                        name='check-circle'
                        size={14}
                    />
                    {report.resolution || 'Resolved'}
                </div>
            )}
        </div>
    );
};

export default ReportCard;
