-- Allow embedded OneBot runtime configuration changes to be recorded in the
-- existing QQBot audit log.

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
        'onebot_settings'
    ));
