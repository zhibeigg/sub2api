-- Record QQBot transport-mode changes in the existing audit log.
-- This preserves all previously accepted actions while allowing BotGo/OneBot switches.

ALTER TABLE qqbot_binding_audit_logs
    DROP CONSTRAINT IF EXISTS qqbot_binding_audit_logs_action_check;

ALTER TABLE qqbot_binding_audit_logs
    ADD CONSTRAINT qqbot_binding_audit_logs_action_check
    CHECK (action IN (
        'prepare',
        'complete',
        'expire',
        'email',
        'notify',
        'unbind',
        'settings',
        'onebot_settings',
        'onebot_request_approval',
        'transport_settings'
    ));
