import manifest from 'manifest';

const prefix = `${manifest.id}_`;

export default {
    RECEIVED_STATUS: `${prefix}received_status`,
    RECEIVED_REPORTS: `${prefix}received_reports`,
    RECEIVED_MEMBERS: `${prefix}received_members`,
    SET_RHS_TAB: `${prefix}set_rhs_tab`,
    OPEN_REPORT_MODAL: `${prefix}open_report_modal`,
    CLOSE_REPORT_MODAL: `${prefix}close_report_modal`,
    OPEN_TIMEOUT_MODAL: `${prefix}open_timeout_modal`,
    CLOSE_TIMEOUT_MODAL: `${prefix}close_timeout_modal`,
    OPEN_CONFIRM_MODAL: `${prefix}open_confirm_modal`,
    CLOSE_CONFIRM_MODAL: `${prefix}close_confirm_modal`,
    SHOW_TOAST: `${prefix}show_toast`,
    HIDE_TOAST: `${prefix}hide_toast`,
} as const;
