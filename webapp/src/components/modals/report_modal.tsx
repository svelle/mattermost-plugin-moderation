import React, {useState} from 'react';
import {useDispatch} from 'react-redux';

import {closeReportModal, submitReport} from '../../actions';
import {REPORT_REASONS} from '../../types';
import type {ReportModalState} from '../../types';
import Icon from '../icons';
import {C} from '../styles';

type Props = {
    modal: ReportModalState;
};

const ReportModal = ({modal}: Props) => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => void;
    const [reason, setReason] = useState<string | null>(null);
    const [note, setNote] = useState('');

    const close = () => dispatch(closeReportModal());
    const isMessage = Boolean(modal.postId);

    return (
        <div
            onClick={close}
            style={{position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.48)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1100}}
        >
            <div
                onClick={(e) => e.stopPropagation()}
                role='dialog'
                aria-label={isMessage ? 'Report this message' : `Report ${modal.targetName}`}
                style={{width: 480, maxWidth: 'calc(100vw - 32px)', maxHeight: '88vh', overflowY: 'auto', background: C.centerBg, color: C.fg1, borderRadius: 8, boxShadow: '0 12px 32px rgba(0,0,0,0.24)', padding: '22px 24px'}}
            >
                <div style={{display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 16}}>
                    <span style={{width: 40, height: 40, borderRadius: 8, background: C.redTint, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
                        <Icon
                            name='flag'
                            size={22}
                            color={C.red500}
                        />
                    </span>
                    <div style={{flex: 1}}>
                        <div style={{fontWeight: 600, fontSize: 19, lineHeight: 1.2}}>
                            {isMessage ? 'Report this message' : `Report ${modal.targetName}`}
                        </div>
                        <div style={{fontSize: 13, color: C.fg3, marginTop: 2}}>
                            {isMessage ? `From ${modal.targetName} in this channel` : 'Flag this member to the moderation team'}
                        </div>
                    </div>
                    <button
                        onClick={close}
                        aria-label='Close'
                        style={{width: 30, height: 30, border: 'none', background: 'transparent', cursor: 'pointer', borderRadius: 4, color: C.fg2}}
                    >
                        <Icon
                            name='close'
                            size={18}
                        />
                    </button>
                </div>

                {Boolean(modal.excerpt) && (
                    <div style={{fontSize: 13, lineHeight: '18px', color: C.fg2, background: C.bg2, borderLeft: `3px solid ${C.border3}`, borderRadius: 6, padding: '10px 12px', marginBottom: 16}}>
                        {modal.excerpt}
                    </div>
                )}

                <div style={{fontSize: 13, fontWeight: 600, marginBottom: 8}}>{'Why are you reporting this?'}</div>
                <div style={{display: 'flex', flexDirection: 'column', gap: 8, marginBottom: 16}}>
                    {REPORT_REASONS.map((r) => {
                        const selected = reason === r.id;
                        return (
                            <button
                                key={r.id}
                                onClick={() => setReason(r.id)}
                                style={{
                                    display: 'flex',
                                    alignItems: 'flex-start',
                                    gap: 11,
                                    padding: '11px 13px',
                                    borderRadius: 6,
                                    border: `2px solid ${selected ? C.buttonBg : C.border2}`,
                                    background: selected ? C.buttonTintFaint : C.centerBg,
                                    cursor: 'pointer',
                                    textAlign: 'left',
                                    fontFamily: 'inherit',
                                }}
                            >
                                <span style={{width: 18, height: 18, borderRadius: '50%', border: `2px solid ${selected ? C.buttonBg : C.border3}`, flexShrink: 0, marginTop: 1, display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
                                    <span style={{width: 8, height: 8, borderRadius: '50%', background: selected ? C.buttonBg : 'transparent'}}/>
                                </span>
                                <span>
                                    <span style={{display: 'block', fontSize: 14, fontWeight: 600, color: C.fg1}}>{r.label}</span>
                                    <span style={{display: 'block', fontSize: 12, color: C.fg3, marginTop: 1}}>{r.desc}</span>
                                </span>
                            </button>
                        );
                    })}
                </div>

                <div style={{fontSize: 13, fontWeight: 600, marginBottom: 6}}>
                    {'Add context '}
                    <span style={{color: C.fg3, fontWeight: 400}}>{'(optional)'}</span>
                </div>
                <textarea
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    placeholder='Anything moderators should know…'
                    rows={3}
                    style={{width: '100%', boxSizing: 'border-box', resize: 'none', padding: '10px 12px', border: `1px solid ${C.border2}`, borderRadius: 6, fontFamily: 'inherit', fontSize: 14, lineHeight: '20px', color: C.fg1, background: C.centerBg, outline: 'none'}}
                />

                <div style={{display: 'flex', alignItems: 'center', gap: 7, marginTop: 14, fontSize: 12, color: C.fg3}}>
                    <Icon
                        name='eye-outline'
                        size={15}
                    />
                    <span>{'Your report is private. The person you report won\'t see who submitted it.'}</span>
                </div>

                <div style={{display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 18}}>
                    <button
                        onClick={close}
                        style={{height: 40, padding: '0 18px', borderRadius: 4, background: 'transparent', color: C.buttonBg, boxShadow: `inset 0 0 0 1px ${C.buttonBg}`, border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: 14, fontFamily: 'inherit'}}
                    >
                        {'Cancel'}
                    </button>
                    <button
                        onClick={() => reason && dispatchThunk(submitReport(modal, reason, note))}
                        disabled={!reason}
                        style={{height: 40, padding: '0 18px', borderRadius: 4, background: reason ? C.buttonBg : C.border2, color: '#fff', border: 'none', cursor: reason ? 'pointer' : 'not-allowed', fontWeight: 600, fontSize: 14, fontFamily: 'inherit'}}
                    >
                        {'Submit report'}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default ReportModal;
