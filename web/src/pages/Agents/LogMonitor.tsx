import { useEffect } from 'react';
import {
    Alert,
    App,
    Button,
    Form,
    Input,
    InputNumber,
    Select,
    Spin,
    Switch,
} from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getLogMonitorConfig, updateLogMonitorConfig } from '@/api/agent';
import type { LogMonitorConfig, LogMonitorRule } from '@/types';
import { getErrorMessage } from '@/lib/utils';
import { PagePanel } from '@/components/PagePanel';

type RuleForm = Omit<LogMonitorRule, 'paths'> & { pathsText: string };
type ConfigForm = {
    enabled: boolean;
    pollIntervalSeconds: number;
    rules: RuleForm[];
};
const newRule = (): RuleForm => ({
    id:
        typeof crypto.randomUUID === 'function'
            ? crypto.randomUUID()
            : `log-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
    name: '',
    enabled: true,
    pathsText: '',
    regex: '(?i)error|only',
    level: 'warning',
    cooldownSeconds: 0,
});

export default function LogMonitor({ agentId }: { agentId: string }) {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [form] = Form.useForm<ConfigForm>();
    const {
        data: config,
        isLoading,
        error,
    } = useQuery({
        queryKey: ['log-monitor', agentId],
        queryFn: async () => (await getLogMonitorConfig(agentId)).data,
        refetchInterval: (query) =>
            query.state.data?.applyStatus === 'pending' ? 3000 : false,
    });
    useEffect(() => {
        if (config && !form.isFieldsTouched())
            form.setFieldsValue({
                enabled: config.enabled,
                pollIntervalSeconds: config.pollIntervalSeconds || 5,
                rules: config.rules.map((rule) => ({
                    ...rule,
                    pathsText: rule.paths.join('\n'),
                })),
            });
    }, [config, form]);
    const save = useMutation({
        mutationFn: async (values: ConfigForm) => {
            const payload: LogMonitorConfig = {
                ...values,
                rules: (values.rules || []).map((rule) => ({
                    id: rule.id,
                    name: rule.name,
                    enabled: rule.enabled,
                    paths: rule.pathsText
                        .split('\n')
                        .map((path) => path.trim())
                        .filter(Boolean),
                    regex: rule.regex,
                    level: rule.level,
                    cooldownSeconds: rule.cooldownSeconds || 0,
                })),
            };
            return updateLogMonitorConfig(agentId, payload);
        },
        onSuccess: async () => {
            message.success('日志配置已保存，探针上线后自动应用');
            form.resetFields();
            await client.invalidateQueries({
                queryKey: ['log-monitor', agentId],
            });
        },
        onError: (err) => message.error(getErrorMessage(err, '保存失败')),
    });
    if (isLoading) return <Spin />;
    if (error)
        return (
            <Alert
                title="日志配置读取失败"
                description={getErrorMessage(error, '请稍后重试')}
                type="error"
                showIcon
            />
        );
    return (
        <PagePanel>
            <div className="space-y-4">
                <Alert
                    title="日志监控"
                    description="探针在本机读取新增日志，命中规则后进入 Pika 告警记录。通知复用告警规则中的“日志告警通知”和现有通知渠道、模板。"
                    type="info"
                    showIcon
                />
                {config?.applyStatus && (
                    <Alert
                        title={
                            config.applyStatus === 'success'
                                ? '配置已应用'
                                : config.applyStatus === 'failed'
                                  ? '配置应用失败'
                                  : '等待探针应用配置'
                        }
                        description={config.applyMessage}
                        type={
                            config.applyStatus === 'failed'
                                ? 'error'
                                : config.applyStatus === 'success'
                                  ? 'success'
                                  : 'info'
                        }
                        showIcon
                    />
                )}
                <Form
                    form={form}
                    layout="vertical"
                    initialValues={{
                        enabled: false,
                        pollIntervalSeconds: 5,
                        rules: [],
                    }}
                    onFinish={(values) => save.mutate(values)}
                >
                    <div className="grid gap-4 sm:grid-cols-2">
                        <Form.Item
                            label="启用日志监控"
                            name="enabled"
                            valuePropName="checked"
                        >
                            <Switch />
                        </Form.Item>
                        <Form.Item
                            label="检查间隔（秒）"
                            name="pollIntervalSeconds"
                            rules={[{ required: true }]}
                        >
                            <InputNumber min={1} max={60} precision={0} />
                        </Form.Item>
                    </div>
                    <Form.List name="rules">
                        {(fields, { add, remove }) => (
                            <div className="space-y-4">
                                {fields.map((field) => (
                                    <div
                                        key={field.key}
                                        className="rounded-lg border border-slate-200 p-4 dark:border-slate-700"
                                    >
                                        <Form.Item
                                            name={[field.name, 'id']}
                                            hidden
                                        >
                                            <Input />
                                        </Form.Item>
                                        <div className="grid gap-4 sm:grid-cols-2">
                                            <Form.Item
                                                label="规则名称"
                                                name={[field.name, 'name']}
                                                rules={[
                                                    {
                                                        required: true,
                                                        message:
                                                            '请输入规则名称',
                                                    },
                                                ]}
                                            >
                                                <Input
                                                    maxLength={128}
                                                    placeholder="Clash Party"
                                                />
                                            </Form.Item>
                                            <Form.Item
                                                label="启用规则"
                                                name={[field.name, 'enabled']}
                                                valuePropName="checked"
                                            >
                                                <Switch />
                                            </Form.Item>
                                        </div>
                                        <Form.Item
                                            label="日志路径"
                                            name={[field.name, 'pathsText']}
                                            rules={[
                                                {
                                                    required: true,
                                                    message: '请输入日志路径',
                                                },
                                            ]}
                                            extra="每行一个绝对路径，支持 * 和 ? 通配符。路径属于当前探针所在机器。"
                                        >
                                            <Input.TextArea
                                                rows={3}
                                                placeholder={
                                                    '~/Library/Application Support/mihomo-party/logs/clash-party-*.log\n~/Library/Application Support/mihomo-party/logs/core-*.log'
                                                }
                                            />
                                        </Form.Item>
                                        <Form.Item
                                            label="匹配表达式"
                                            name={[field.name, 'regex']}
                                            rules={[
                                                {
                                                    required: true,
                                                    message: '请输入正则表达式',
                                                },
                                            ]}
                                            extra="使用 Go 正则语法。例如 (?i)error|only 表示忽略大小写，任一关键词命中即告警。"
                                        >
                                            <Input maxLength={2048} />
                                        </Form.Item>
                                        <div className="grid gap-4 sm:grid-cols-2">
                                            <Form.Item
                                                label="告警级别"
                                                name={[field.name, 'level']}
                                                rules={[{ required: true }]}
                                            >
                                                <Select
                                                    options={[
                                                        {
                                                            value: 'info',
                                                            label: '信息',
                                                        },
                                                        {
                                                            value: 'warning',
                                                            label: '警告',
                                                        },
                                                        {
                                                            value: 'critical',
                                                            label: '严重',
                                                        },
                                                    ]}
                                                />
                                            </Form.Item>
                                            <Form.Item
                                                label="冷却时间（秒）"
                                                name={[
                                                    field.name,
                                                    'cooldownSeconds',
                                                ]}
                                                extra="0 表示逐条告警；大于 0 时，规则冷却期间的命中不再生成告警。"
                                            >
                                                <InputNumber
                                                    min={0}
                                                    max={86400}
                                                    precision={0}
                                                />
                                            </Form.Item>
                                        </div>
                                        <Button
                                            danger
                                            onClick={() => remove(field.name)}
                                        >
                                            删除规则
                                        </Button>
                                    </div>
                                ))}
                                <Button
                                    disabled={fields.length >= 32}
                                    onClick={() => add(newRule())}
                                >
                                    添加日志规则
                                </Button>
                            </div>
                        )}
                    </Form.List>
                    <div className="mt-4">
                        <Button
                            type="primary"
                            htmlType="submit"
                            loading={save.isPending}
                        >
                            保存配置
                        </Button>
                    </div>
                </Form>
            </div>
        </PagePanel>
    );
}
