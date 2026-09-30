import { useEffect, useRef, useState } from 'react';
import {
    Alert,
    App,
    Button,
    Form,
    InputNumber,
    Popconfirm,
    Space,
    Spin,
    Switch,
    Tag,
} from 'antd';
import type { TableProps } from 'antd';
import { FileText, Plus, Save } from 'lucide-react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getLogMonitorConfig, updateLogMonitorConfig } from '@/api/agent';
import type { LogMonitorConfig, LogMonitorRule } from '@/types';
import { getErrorMessage } from '@/lib/utils';
import { AdminDataTable } from '@/components/AdminDataTable';
import LogMonitorRuleModal from './LogMonitorRuleModal';

type ConfigFields = { enabled: boolean; pollIntervalSeconds: number };
const levels: Record<string, { color: string; text: string }> = {
    info: { color: 'blue', text: '信息' },
    warning: { color: 'orange', text: '警告' },
    critical: { color: 'red', text: '严重' },
};

export default function LogMonitor({ agentId }: { agentId: string }) {
    const { message } = App.useApp();
    const client = useQueryClient();
    const [form] = Form.useForm<ConfigFields>();
    const [rules, setRules] = useState<LogMonitorRule[]>([]);
    const [dirty, setDirty] = useState(false);
    const [modalOpen, setModalOpen] = useState(false);
    const [editingRule, setEditingRule] = useState<LogMonitorRule>();
    const loadedAgent = useRef(agentId);
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
        if (loadedAgent.current === agentId) return;
        loadedAgent.current = agentId;
        setDirty(false);
        setRules([]);
        setModalOpen(false);
        form.resetFields();
    }, [agentId, form]);

    useEffect(() => {
        if (!config || dirty) return;
        form.setFieldsValue({
            enabled: config.enabled,
            pollIntervalSeconds: config.pollIntervalSeconds || 5,
        });
        setRules(config.rules);
    }, [config, dirty, form]);

    const save = useMutation({
        mutationFn: async (values: ConfigFields) => {
            const payload: LogMonitorConfig = { ...values, rules };
            await updateLogMonitorConfig(agentId, payload);
            return payload;
        },
        onSuccess: (payload) => {
            // Keep the submitted values visible while the probe applies the saved configuration.
            client.setQueryData<LogMonitorConfig>(['log-monitor', agentId], {
                ...config,
                ...payload,
                applyStatus: 'pending',
                applyMessage: '',
            });
            setDirty(false);
            message.success('配置已保存');
            client.invalidateQueries({ queryKey: ['log-monitor', agentId] });
        },
        onError: (err) => message.error(getErrorMessage(err, '配置保存失败')),
    });

    const changeRules = (next: LogMonitorRule[]) => {
        setRules(next);
        setDirty(true);
    };
    const edit = (rule?: LogMonitorRule) => {
        setEditingRule(rule);
        setModalOpen(true);
    };
    const submitRule = (rule: LogMonitorRule) => {
        changeRules(
            editingRule
                ? rules.map((current) =>
                      current.id === editingRule.id ? rule : current,
                  )
                : [...rules, rule],
        );
        setModalOpen(false);
    };

    const columns: TableProps<LogMonitorRule>['columns'] = [
        { title: '规则名称', dataIndex: 'name', width: 170 },
        {
            title: '日志路径',
            dataIndex: 'paths',
            width: 300,
            render: (paths: string[]) => (
                <div
                    className="max-w-[300px] truncate"
                    title={paths.join('\n')}
                >
                    {paths[0]}
                    {paths.length > 1 ? ` 等 ${paths.length} 个路径` : ''}
                </div>
            ),
        },
        {
            title: '匹配表达式',
            dataIndex: 'regex',
            width: 180,
            ellipsis: true,
            render: (regex: string) => <code>{regex}</code>,
        },
        {
            title: '告警级别',
            dataIndex: 'level',
            width: 95,
            render: (level: string) => (
                <Tag color={levels[level]?.color}>
                    {levels[level]?.text || level}
                </Tag>
            ),
        },
        {
            title: '冷却时间',
            dataIndex: 'cooldownSeconds',
            width: 95,
            render: (seconds: number) => (seconds ? `${seconds} 秒` : '无'),
        },
        {
            title: '启用',
            dataIndex: 'enabled',
            width: 75,
            render: (enabled: boolean, rule) => (
                <Switch
                    size="small"
                    checked={enabled}
                    aria-label={`启用规则 ${rule.name}`}
                    disabled={save.isPending}
                    onChange={(value) =>
                        changeRules(
                            rules.map((current) =>
                                current.id === rule.id
                                    ? { ...current, enabled: value }
                                    : current,
                            ),
                        )
                    }
                />
            ),
        },
        {
            title: '操作',
            width: 130,
            fixed: 'right',
            render: (_, rule) => (
                <Space size="small">
                    <Button
                        type="link"
                        size="small"
                        disabled={save.isPending}
                        onClick={() => edit(rule)}
                    >
                        编辑
                    </Button>
                    <Popconfirm
                        title="删除这条规则？"
                        description="保存配置后将从探针移除。"
                        okText="删除"
                        cancelText="取消"
                        onConfirm={() =>
                            changeRules(
                                rules.filter(
                                    (current) => current.id !== rule.id,
                                ),
                            )
                        }
                    >
                        <Button
                            type="link"
                            size="small"
                            danger
                            disabled={save.isPending}
                        >
                            删除
                        </Button>
                    </Popconfirm>
                </Space>
            ),
        },
    ];

    return (
        <div>
            <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2 text-[15px] font-semibold text-[#1f2329] dark:text-[#e6e8ec]">
                    <FileText size={18} />
                    <span>日志监控配置</span>
                    {dirty && <Tag color="warning">未保存</Tag>}
                </div>
                <Button
                    type="primary"
                    icon={<Save size={16} />}
                    loading={save.isPending}
                    disabled={isLoading || !!error || modalOpen}
                    onClick={() => form.submit()}
                >
                    保存配置
                </Button>
            </div>
            {isLoading ? (
                <div className="py-12 text-center">
                    <Spin />
                </div>
            ) : error ? (
                <div className="mt-4">
                    <Alert
                        title="日志配置读取失败"
                        description={getErrorMessage(error, '请稍后重试')}
                        type="error"
                        showIcon
                    />
                </div>
            ) : (
                <div className="mt-4 space-y-4">
                    <Form
                        form={form}
                        layout="vertical"
                        initialValues={{
                            enabled: false,
                            pollIntervalSeconds: 5,
                        }}
                        disabled={save.isPending}
                        onValuesChange={() => setDirty(true)}
                        onFinish={(values) => save.mutate(values)}
                    >
                        <div className="grid gap-x-6 sm:grid-cols-2">
                            <Form.Item
                                label="启用日志监控"
                                name="enabled"
                                valuePropName="checked"
                                extra="命中日志进入告警记录，通知沿用已有告警规则与模板。"
                            >
                                <Switch
                                    checkedChildren="已启用"
                                    unCheckedChildren="已禁用"
                                />
                            </Form.Item>
                            <Form.Item
                                label="检查间隔（秒）"
                                name="pollIntervalSeconds"
                                rules={[
                                    {
                                        required: true,
                                        message: '请输入检查间隔',
                                    },
                                ]}
                            >
                                <InputNumber
                                    min={1}
                                    max={60}
                                    precision={0}
                                    style={{ width: 140 }}
                                />
                            </Form.Item>
                        </div>
                    </Form>
                    <div>
                        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
                            <div className="text-sm font-medium">
                                日志规则（{rules.length}）
                            </div>
                            <Button
                                icon={<Plus size={16} />}
                                disabled={rules.length >= 32 || save.isPending}
                                onClick={() => edit()}
                            >
                                添加规则
                            </Button>
                        </div>
                        <AdminDataTable<LogMonitorRule>
                            rowKey="id"
                            columns={columns}
                            dataSource={rules}
                            pagination={false}
                            scroll={{ x: 1060 }}
                            locale={{ emptyText: '暂未配置日志规则' }}
                        />
                        <p className="mt-2 text-xs text-[#646a73] dark:text-[#9ba1ab]">
                            新增、编辑或删除规则后，点击上方“保存配置”应用到探针。
                        </p>
                    </div>
                    {!dirty && config?.applyStatus && (
                        <Alert
                            title={
                                config.applyStatus === 'success'
                                    ? '配置应用成功'
                                    : config.applyStatus === 'failed'
                                      ? '配置应用失败'
                                      : '等待探针应用配置'
                            }
                            description={
                                config.applyStatus === 'failed'
                                    ? config.applyMessage
                                    : undefined
                            }
                            type={
                                config.applyStatus === 'success'
                                    ? 'success'
                                    : config.applyStatus === 'failed'
                                      ? 'error'
                                      : 'info'
                            }
                            showIcon
                        />
                    )}
                </div>
            )}
            <LogMonitorRuleModal
                open={modalOpen}
                rule={editingRule}
                onCancel={() => setModalOpen(false)}
                onSubmit={submitRule}
            />
        </div>
    );
}
