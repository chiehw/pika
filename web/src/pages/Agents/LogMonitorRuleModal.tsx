import { useEffect } from 'react';
import { Form, Input, InputNumber, Modal, Select, Switch } from 'antd';
import type { LogMonitorRule } from '@/types';

type RuleFields = Omit<LogMonitorRule, 'paths'> & { pathsText: string };

interface Props {
    open: boolean;
    rule?: LogMonitorRule;
    onCancel: () => void;
    onSubmit: (rule: LogMonitorRule) => void;
}

export default function LogMonitorRuleModal({
    open,
    rule,
    onCancel,
    onSubmit,
}: Props) {
    const [form] = Form.useForm<RuleFields>();

    useEffect(() => {
        if (!open) return;
        form.resetFields();
        form.setFieldsValue(
            rule
                ? { ...rule, pathsText: rule.paths.join('\n') }
                : {
                      id:
                          typeof crypto.randomUUID === 'function'
                              ? crypto.randomUUID()
                              : `log-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
                      name: '',
                      enabled: true,
                      pathsText: '',
                      regex: '(?i)error',
                      level: 'warning',
                      cooldownSeconds: 0,
                  },
        );
    }, [open, rule, form]);

    const finish = (values: RuleFields) => {
        onSubmit({
            id: values.id,
            name: values.name.trim(),
            enabled: values.enabled,
            paths: values.pathsText
                .split('\n')
                .map((path) => path.trim())
                .filter(Boolean),
            regex: values.regex,
            level: values.level,
            cooldownSeconds: values.cooldownSeconds || 0,
        });
    };

    return (
        <Modal
            title={rule ? '编辑日志规则' : '新增日志规则'}
            open={open}
            onOk={() => form.submit()}
            onCancel={onCancel}
            okText="确定"
            cancelText="取消"
            width={640}
            destroyOnHidden
        >
            <Form
                form={form}
                layout="vertical"
                onFinish={finish}
                preserve={false}
            >
                <Form.Item name="id" hidden>
                    <Input />
                </Form.Item>
                <div className="grid gap-x-4 sm:grid-cols-2">
                    <Form.Item
                        label="规则名称"
                        name="name"
                        rules={[
                            {
                                required: true,
                                whitespace: true,
                                message: '请输入规则名称',
                            },
                        ]}
                    >
                        <Input
                            maxLength={128}
                            placeholder="例如：应用错误日志"
                        />
                    </Form.Item>
                    <Form.Item
                        label="启用规则"
                        name="enabled"
                        valuePropName="checked"
                    >
                        <Switch
                            checkedChildren="已启用"
                            unCheckedChildren="已禁用"
                        />
                    </Form.Item>
                </div>
                <Form.Item
                    label="日志路径"
                    name="pathsText"
                    extra="每行一个探针本机的完整路径，支持 * 和 ? 通配符。"
                    rules={[
                        {
                            required: true,
                            whitespace: true,
                            message: '请输入日志路径',
                        },
                        {
                            validator: (_, value: string) => {
                                const paths = (value || '')
                                    .split('\n')
                                    .map((path) => path.trim())
                                    .filter(Boolean);
                                if (paths.length > 16)
                                    return Promise.reject(
                                        new Error('每条规则最多配置 16 个路径'),
                                    );
                                return Promise.resolve();
                            },
                        },
                    ]}
                >
                    <Input.TextArea
                        rows={3}
                        placeholder="请输入日志文件的完整路径，每行一个"
                    />
                </Form.Item>
                <Form.Item
                    label="匹配表达式"
                    name="regex"
                    rules={[{ required: true, message: '请输入匹配表达式' }]}
                    extra="使用 Go 正则语法；(?i)error 表示忽略大小写匹配 error。"
                >
                    <Input maxLength={2048} />
                </Form.Item>
                <div className="grid gap-x-4 sm:grid-cols-2">
                    <Form.Item
                        label="告警级别"
                        name="level"
                        rules={[{ required: true }]}
                    >
                        <Select
                            options={[
                                { value: 'info', label: '信息' },
                                { value: 'warning', label: '警告' },
                                { value: 'critical', label: '严重' },
                            ]}
                        />
                    </Form.Item>
                    <Form.Item
                        label="冷却时间（秒）"
                        name="cooldownSeconds"
                        extra="0 表示逐条告警；冷却期间的命中不再生成告警。"
                    >
                        <InputNumber
                            min={0}
                            max={86400}
                            precision={0}
                            style={{ width: '100%' }}
                        />
                    </Form.Item>
                </div>
            </Form>
        </Modal>
    );
}
