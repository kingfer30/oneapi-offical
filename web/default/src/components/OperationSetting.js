import React, { useEffect, useState } from 'react';
import { Divider, Form, Grid, Header } from 'semantic-ui-react';
import { API, showError, showSuccess, timestamp2string, verifyJSON } from '../helpers';

const OperationSetting = () => {
  let now = new Date();
  let [inputs, setInputs] = useState({
    QuotaForNewUser: 0,
    QuotaForInviter: 0,
    QuotaForInvitee: 0,
    QuotaRemindThreshold: 0,
    PreConsumedQuota: 0,
    ModelRatio: '',
    CompletionRatio: '',
    GroupRatio: '',
    TopUpLink: '',
    ChatLink: '',
    QuotaPerUnit: 0,
    AutomaticDisableChannelEnabled: '',
    AutomaticEnableChannelEnabled: '',
    ChannelDisableThreshold: 0,
    LogConsumeEnabled: '',
    DisplayInCurrencyEnabled: '',
    DisplayTokenStatEnabled: '',
    ApproximateTokenEnabled: '',
    RetryTimes: 0,
    PoolMode: 'random',
    AutoAddChannelEnabled: '',
    CacheChannelEnabled: '',
    GeminiNewEnabled: '',
    GeminiUploadImageEnabled: '',
    QuotaForAddChannel: 0,
  });
  const [originInputs, setOriginInputs] = useState({});
  let [loading, setLoading] = useState(false);
  let [btnLoading, setBtnLoading] = useState(false);
  let [historyTimestamp, setHistoryTimestamp] = useState(timestamp2string(now.getTime() / 1000 - 30 * 24 * 3600)); // a month ago

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      let newInputs = {};
      data.forEach((item) => {
        if (item.key === 'ModelRatio' || item.key === 'GroupRatio' || item.key === 'CompletionRatio') {
          item.value = JSON.stringify(JSON.parse(item.value), null, 2);
        }
        if (item.value === '{}') {
          item.value = '';
        }
        newInputs[item.key] = item.value;
      });
      setInputs(newInputs);
      setOriginInputs(newInputs);
    } else {
      showError(message);
    }
  };

  useEffect(() => {
    getOptions().then();
  }, []);

  const updateOption = async (key, value) => {
    setLoading(true);
    if (key.endsWith('Enabled')) {
      value = inputs[key] === 'true' ? 'false' : 'true';
    }
    const res = await API.put('/api/option/', {
      key,
      value
    });
    const { success, message } = res.data;
    if (success) {
      setInputs((inputs) => ({ ...inputs, [key]: value }));
    } else {
      showError(message);
    }
    setLoading(false);
  };
  const updateChannelAbilities = async () => {
    setBtnLoading(true);
    const res = await API.post('/api/channel/update_abilities', {});
    const { success, message } = res.data;
    if (success) {
      showSuccess("渠道模型已更新");
    } else {
      showError(message);
    }
    setBtnLoading(false);
  };

  const handleInputChange = async (e, { name, value }) => {
    if (name.endsWith('Enabled')) {
      await updateOption(name, value);
    } else {
      setInputs((inputs) => ({ ...inputs, [name]: value }));
    }
  };

  const submitConfig = async (group) => {
    switch (group) {
      case 'monitor':
        if (originInputs['ChannelDisableThreshold'] !== inputs.ChannelDisableThreshold) {
          await updateOption('ChannelDisableThreshold', inputs.ChannelDisableThreshold);
        }
        if (originInputs['QuotaRemindThreshold'] !== inputs.QuotaRemindThreshold) {
          await updateOption('QuotaRemindThreshold', inputs.QuotaRemindThreshold);
        }
        break;
      case 'ratio':
        if (originInputs['ModelRatio'] !== inputs.ModelRatio) {
          if (!verifyJSON(inputs.ModelRatio)) {
            showError('模型倍率不是合法的 JSON 字符串');
            return;
          }
          await updateOption('ModelRatio', inputs.ModelRatio);
        }
        if (originInputs['GroupRatio'] !== inputs.GroupRatio) {
          if (!verifyJSON(inputs.GroupRatio)) {
            showError('分组倍率不是合法的 JSON 字符串');
            return;
          }
          await updateOption('GroupRatio', inputs.GroupRatio);
        }
        if (originInputs['CompletionRatio'] !== inputs.CompletionRatio) {
          if (!verifyJSON(inputs.CompletionRatio)) {
            showError('补全倍率不是合法的 JSON 字符串');
            return;
          }
          await updateOption('CompletionRatio', inputs.CompletionRatio);
        }
        break;
      case 'quota':
        if (originInputs['QuotaForNewUser'] !== inputs.QuotaForNewUser) {
          await updateOption('QuotaForNewUser', inputs.QuotaForNewUser);
        }
        if (originInputs['QuotaForInvitee'] !== inputs.QuotaForInvitee) {
          await updateOption('QuotaForInvitee', inputs.QuotaForInvitee);
        }
        if (originInputs['QuotaForInviter'] !== inputs.QuotaForInviter) {
          await updateOption('QuotaForInviter', inputs.QuotaForInviter);
        }
        if (originInputs['PreConsumedQuota'] !== inputs.PreConsumedQuota) {
          await updateOption('PreConsumedQuota', inputs.PreConsumedQuota);
        }
        break;
      case 'general':
        if (originInputs['TopUpLink'] !== inputs.TopUpLink) {
          await updateOption('TopUpLink', inputs.TopUpLink);
        }
        if (originInputs['ChatLink'] !== inputs.ChatLink) {
          await updateOption('ChatLink', inputs.ChatLink);
        }
        if (originInputs['QuotaPerUnit'] !== inputs.QuotaPerUnit) {
          await updateOption('QuotaPerUnit', inputs.QuotaPerUnit);
        }
        if (originInputs['RetryTimes'] !== inputs.RetryTimes) {
          await updateOption('RetryTimes', inputs.RetryTimes);
        }
        break;
        case 'channel':
          if (originInputs['QuotaForAddChannel'] !== inputs.QuotaForAddChannel) {
            await updateOption('QuotaForAddChannel', inputs.QuotaForAddChannel);
          }
          if (originInputs['PoolMode'] !== inputs.PoolMode) {
            await updateOption('PoolMode', inputs.PoolMode);
          }
          break;
    }
  };

  const deleteHistoryLogs = async () => {
    console.log(inputs);
    const res = await API.delete(`/api/log/?target_timestamp=${Date.parse(historyTimestamp) / 1000}`);
    const { success, message, data } = res.data;
    if (success) {
      showSuccess(`${data} 条日志已清理！`);
      return;
    }
    showError('日志清理失败：' + message);
  };
  
  const poolOptions = [
    { key: "random", text: '普通随机模式', value: 'random' },
    { key: "polling", text: '最优轮询模式', value: 'polling' },
  ]

  return (
    <Grid columns={1}>
      <Grid.Column>
        <Form loading={loading}>
          <Header as='h3'>
            通用设置
          </Header>
          <Form.Group widths={4}>
            <Form.Input
              label='充值链接'
              name='TopUpLink'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.TopUpLink}
              type='link'
              placeholder='例如发卡网站的购买链接'
            />
            <Form.Input
              label='聊天页面链接'
              name='ChatLink'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.ChatLink}
              type='link'
              placeholder='例如 ChatGPT Next Web 的部署地址'
            />
            <Form.Input
              label='单位美元额度'
              name='QuotaPerUnit'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.QuotaPerUnit}
              type='number'
              step='0.01'
              placeholder='一单位货币能兑换的额度'
            />
            <Form.Input
              label='失败重试次数'
              name='RetryTimes'
              type={'number'}
              step='1'
              min='0'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.RetryTimes}
              placeholder='失败重试次数'
            />
          </Form.Group>
          <Form.Group inline>
            <Form.Checkbox
              checked={inputs.DisplayInCurrencyEnabled === 'true'}
              label='以货币形式显示额度'
              name='DisplayInCurrencyEnabled'
              onChange={handleInputChange}
            />
            <Form.Checkbox
              checked={inputs.DisplayTokenStatEnabled === 'true'}
              label='Billing 相关 API 显示令牌额度而非用户额度'
              name='DisplayTokenStatEnabled'
              onChange={handleInputChange}
            />
            <Form.Checkbox
              checked={inputs.ApproximateTokenEnabled === 'true'}
              label='使用近似的方式估算 token 数以减少计算量'
              name='ApproximateTokenEnabled'
              onChange={handleInputChange}
            />
          </Form.Group>
          <Form.Button onClick={() => {
            submitConfig('general').then();
          }}>保存通用设置</Form.Button>
          <Divider />
          <Header as='h3'>
              渠道设置
            </Header>
            <Form.Button onClick={() => {
              updateChannelAbilities().then();
            }} loading={btnLoading}>一键更新渠道模型</Form.Button>
            <Form.Group inline>
              <Form.Select
                name='PoolMode'
                label='命中模式'
                options={poolOptions}
                placeholder='选择命中模式'
                value={inputs.PoolMode}
                onChange={handleInputChange}
              />
            </Form.Group>
            <Form.Group inline>
              <Form.Checkbox
                checked={inputs.CacheChannelEnabled === 'true'}
                label='启用Redis缓存渠道信息'
                name='CacheChannelEnabled'
                onChange={handleInputChange}
              /><i className="warning circle icon" style={{ display: 'flex', alignItems: 'center' }}></i>启用后渠道信息将使用Redis引擎进行缓存, 而不缓存在内存中, 可通过手动移除Redis的渠道缓存以及时更新信息
            </Form.Group>
            <Form.Group inline>
              <Form.Checkbox
                checked={inputs.AutoAddChannelEnabled === 'true'}
                label='启用自动补货'
                name='AutoAddChannelEnabled'
                onChange={handleInputChange}
              /><i className="warning circle icon" style={{ display: 'flex', alignItems: 'center' }}></i>根据设定渠道可用数量, 当渠道实际可用数量小于设定值时, 自动将备用池的渠道启动
            </Form.Group>
            <Form.Group inline>
              <Form.Checkbox
                checked={inputs.GeminiNewEnabled === 'true'}
                label='Gemini启用新版逻辑'
                name='GeminiNewEnabled'
                onChange={handleInputChange}
              /><i className="warning circle icon" style={{ display: 'flex', alignItems: 'center' }}></i>启用后Gemini类型将采用新版逻辑
            </Form.Group>
            <Form.Group inline>
              <Form.Checkbox
                checked={inputs.GeminiUploadImageEnabled === 'true'}
                label='开启图片上传至Gemini'
                name='GeminiUploadImageEnabled'
                onChange={handleInputChange}
              /><i className="warning circle icon" style={{ display: 'flex', alignItems: 'center' }}></i>启用后识图对应的图片都会上传到Gemini
            </Form.Group>
            <Form.Group inline>
              <Form.Input
                label='补货阈值'
                name='QuotaForAddChannel'
                onChange={handleInputChange}
                autoComplete='new-password'
                value={inputs.QuotaForAddChannel}
                type='number'
                min='0'
                placeholder='低于此阈值时, 对应渠道类型自动将待激活的渠道激活'
              />
            </Form.Group>
            <Form.Button onClick={() => {
              submitConfig('channel').then();
            }}>保存渠道设置</Form.Button>
            <Divider />
          <Header as='h3'>
            日志设置
          </Header>
          <Form.Group inline>
            <Form.Checkbox
              checked={inputs.LogConsumeEnabled === 'true'}
              label='启用额度消费日志记录'
              name='LogConsumeEnabled'
              onChange={handleInputChange}
            />
          </Form.Group>
          <Form.Group widths={4}>
            <Form.Input label='目标时间' value={historyTimestamp} type='datetime-local'
              name='history_timestamp'
              onChange={(e, { name, value }) => {
                setHistoryTimestamp(value);
              }} />
          </Form.Group>
          <Form.Button onClick={() => {
            deleteHistoryLogs().then();
          }}>清理历史日志</Form.Button>
          <Divider />
          <Header as='h3'>
            监控设置
          </Header>
          <Form.Group widths={3}>
            <Form.Input
              label='最长响应时间'
              name='ChannelDisableThreshold'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.ChannelDisableThreshold}
              type='number'
              min='0'
              placeholder='单位秒，当运行渠道全部测试时，超过此时间将自动禁用渠道'
            />
            <Form.Input
              label='额度提醒阈值'
              name='QuotaRemindThreshold'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.QuotaRemindThreshold}
              type='number'
              min='0'
              placeholder='低于此额度时将发送邮件提醒用户'
            />
            <Form.Input
              label='补货阈值'
              name='QuotaForAddChannel'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.QuotaForAddChannel}
              type='number'
              min='0'
              placeholder='低于此阈值时, 对应渠道类型自动将待激活的渠道激活'
            />
          </Form.Group>
          <Form.Group inline>
            <Form.Checkbox
              checked={inputs.AutomaticDisableChannelEnabled === 'true'}
              label='失败时自动禁用渠道'
              name='AutomaticDisableChannelEnabled'
              onChange={handleInputChange}
            />
            <Form.Checkbox
              checked={inputs.AutomaticEnableChannelEnabled === 'true'}
              label='成功时自动启用渠道'
              name='AutomaticEnableChannelEnabled'
              onChange={handleInputChange}
            />
          </Form.Group>
          <Form.Button onClick={() => {
            submitConfig('monitor').then();
          }}>保存监控设置</Form.Button>
          <Divider />
          <Header as='h3'>
            额度设置
          </Header>
          <Form.Group widths={4}>
            <Form.Input
              label='新用户初始额度'
              name='QuotaForNewUser'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.QuotaForNewUser}
              type='number'
              min='0'
              placeholder='例如：100'
            />
            <Form.Input
              label='请求预扣费额度'
              name='PreConsumedQuota'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.PreConsumedQuota}
              type='number'
              min='0'
              placeholder='请求结束后多退少补'
            />
            <Form.Input
              label='邀请新用户奖励额度'
              name='QuotaForInviter'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.QuotaForInviter}
              type='number'
              min='0'
              placeholder='例如：2000'
            />
            <Form.Input
              label='新用户使用邀请码奖励额度'
              name='QuotaForInvitee'
              onChange={handleInputChange}
              autoComplete='new-password'
              value={inputs.QuotaForInvitee}
              type='number'
              min='0'
              placeholder='例如：1000'
            />
          </Form.Group>
          <Form.Button onClick={() => {
            submitConfig('quota').then();
          }}>保存额度设置</Form.Button>
          <Divider />
          <Header as='h3'>
            倍率设置
          </Header>
          <Form.Group widths='equal'>
            <Form.TextArea
              label='模型倍率'
              name='ModelRatio'
              onChange={handleInputChange}
              style={{ minHeight: 250, fontFamily: 'JetBrains Mono, Consolas' }}
              autoComplete='new-password'
              value={inputs.ModelRatio}
              placeholder='为一个 JSON 文本，键为模型名称，值为倍率'
            />
          </Form.Group>
          <Form.Group widths='equal'>
            <Form.TextArea
              label='补全倍率'
              name='CompletionRatio'
              onChange={handleInputChange}
              style={{ minHeight: 250, fontFamily: 'JetBrains Mono, Consolas' }}
              autoComplete='new-password'
              value={inputs.CompletionRatio}
              placeholder='为一个 JSON 文本，键为模型名称，值为倍率，此处的倍率设置是模型补全倍率相较于提示倍率的比例，使用该设置可强制覆盖 One API 的内部比例'
            />
          </Form.Group>
          <Form.Group widths='equal'>
            <Form.TextArea
              label='分组倍率'
              name='GroupRatio'
              onChange={handleInputChange}
              style={{ minHeight: 250, fontFamily: 'JetBrains Mono, Consolas' }}
              autoComplete='new-password'
              value={inputs.GroupRatio}
              placeholder='为一个 JSON 文本，键为分组名称，值为倍率'
            />
          </Form.Group>
          <Form.Button onClick={() => {
            submitConfig('ratio').then();
          }}>保存倍率设置</Form.Button>
        </Form>
      </Grid.Column>
    </Grid>
  );
};

export default OperationSetting;
