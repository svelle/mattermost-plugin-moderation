import React, {useCallback, useState} from 'react';
import {useDispatch} from 'react-redux';

import {closeTimeoutModal, submitTimeout} from '../../actions';
import {TIMEOUT_DURATIONS} from '../../types';
import type {TimeoutModalState} from '../../types';
import {useEscape} from '../hooks';
import Icon from '../icons';
import {C} from '../styles';

type Props = {
    modal: TimeoutModalState;
};

const TimeoutModal = ({modal}: Props) => {
    const dispatch = useDispatch();
    const dispatchThunk = dispatch as (action: unknown) => Promise<unknown>;
    const [duration, setDuration] = useState<string | null>(null);
    const [submitting, setSubmitting] = useState(false);

    const close = useCallback(() => dispatch(closeTimeoutModal()), [dispatch]);
    useEscape(close);

    const canSubmit = Boolean(duration) && !submitting;
    const submit = async () => {
        if (!duration || submitting) {
            return;
        }
        setSubmitting(true);
        try {
            await dispatchThunk(submitTimeout(modal, duration));
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div
            onClick={close}
            style={{position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.48)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1100}}
        >
            <div
                onClick={(e) => e.stopPropagation()}
                role='dialog'
                aria-modal='true'
                aria-label={`Timeout ${modal.targetName}`}
                style={{width: 440, maxWidth: 'calc(100vw - 32px)', background: C.centerBg, color: C.fg1, borderRadius: 8, boxShadow: '0 12px 32px rgba(0,0,0,0.24)', padding: '22px 24px'}}
            >
                <div style={{display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 18}}>
                    <span style={{width: 40, height: 40, borderRadius: 8, background: C.yellowTint, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
                        <Icon
                            name='clock'
                            size={22}
                            color={C.yellow600}
                        />
                    </span>
                    <div style={{flex: 1}}>
                        <div style={{fontWeight: 600, fontSize: 19, lineHeight: 1.2}}>{'Timeout '}{modal.targetName}</div>
                        <div style={{fontSize: 13, color: C.fg3, marginTop: 2}}>
                            {'They stay in the channel but can\'t post or react until the timeout ends.'}
                        </div>
                    </div>
                </div>

                <div style={{fontSize: 13, fontWeight: 600, marginBottom: 8}}>{'Duration'}</div>
                <div style={{display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, marginBottom: 18}}>
                    {TIMEOUT_DURATIONS.map((d) => {
                        const selected = duration === d.id;
                        return (
                            <button
                                key={d.id}
                                onClick={() => setDuration(d.id)}
                                style={{
                                    height: 44,
                                    borderRadius: 6,
                                    border: `2px solid ${selected ? C.buttonBg : C.border2}`,
                                    background: selected ? C.buttonTintFaint : C.centerBg,
                                    cursor: 'pointer',
                                    fontFamily: 'inherit',
                                    fontSize: 14,
                                    fontWeight: 600,
                                    color: selected ? C.buttonBg : C.fg1,
                                }}
                            >
                                {d.label}
                            </button>
                        );
                    })}
                </div>

                <div style={{display: 'flex', justifyContent: 'flex-end', gap: 8}}>
                    <button
                        onClick={close}
                        style={{height: 40, padding: '0 18px', borderRadius: 4, background: 'transparent', color: C.buttonBg, boxShadow: `inset 0 0 0 1px ${C.buttonBg}`, border: 'none', cursor: 'pointer', fontWeight: 600, fontSize: 14, fontFamily: 'inherit'}}
                    >
                        {'Cancel'}
                    </button>
                    <button
                        onClick={submit}
                        disabled={!canSubmit}
                        style={{height: 40, padding: '0 18px', borderRadius: 4, background: canSubmit ? C.buttonBg : C.border2, color: '#fff', border: 'none', cursor: canSubmit ? 'pointer' : 'not-allowed', fontWeight: 600, fontSize: 14, fontFamily: 'inherit'}}
                    >
                        {submitting ? 'Applying…' : 'Apply timeout'}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default TimeoutModal;
