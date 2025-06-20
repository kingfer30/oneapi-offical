import React, { useEffect, useState } from 'react';
import { Button, Form, Header, Message, Segment } from 'semantic-ui-react';
import { useNavigate, useParams } from 'react-router-dom';
import { API, copy, showError, showSuccess, timestamp2string } from '../../helpers';
import { renderQuotaWithPrompt } from '../../helpers/render';

const EditToken = () => {
  const params = useParams();
  const tokenId = params.id;
  const isEdit = tokenId !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const [modelOptions, setModelOptions] = useState([]);
  const originInputs = {
    name: '',
    remain_quota: isEdit ? 0 : 500000,
    expired_time: -1,
    unlimited_quota: false,
    batch_number: 1,
    recharge_quota: 0,
    rpm_limit: 600,
    dpm_limit: 60,
    tpm_limit: 60,
    email: '',
    custom_contact: '',
    moderations_enable: false,
    models: [],
    subnet: "",
  };
  const [inputs, setInputs] = useState(originInputs);
  const { name, remain_quota, expired_time, unlimited_quota, moderations_enable, batch_number, recharge_quota, rpm_limit, dpm_limit, tpm_limit, email, custom_contact } = inputs;
  const navigate = useNavigate();
  const handleInputChange = (e, { name, value }) => {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };
  const handleCancel = () => {
    navigate('/token');
  };
  const setExpiredTime = (month, day, hour, minute) => {
    let now = new Date();
    let timestamp = now.getTime() / 1000;
    let seconds = month * 30 * 24 * 60 * 60;
    seconds += day * 24 * 60 * 60;
    seconds += hour * 60 * 60;
    seconds += minute * 60;
    if (seconds !== 0) {
      timestamp += seconds;
      setInputs({ ...inputs, expired_time: timestamp2string(timestamp) });
    } else {
      setInputs({ ...inputs, expired_time: -1 });
    }
  };

  const setUnlimitedQuota = () => {
    setInputs({ ...inputs, unlimited_quota: !unlimited_quota });
  };
  
  const setModerationsEnable = () => {
    setInputs({ ...inputs, moderations_enable: !moderations_enable });
  };

  const loadToken = async () => {
    let res = await API.get(`/api/token/${tokenId}`);
    const { success, message, data } = res.data;
    if (success) {
      if (data.expired_time !== -1) {
        data.expired_time = timestamp2string(data.expired_time);
      }
      if (data.models === '') {
        data.models = [];
      } else {
        data.models = data.models.split(',');
      }
      setInputs(data);
    } else {
      showError(message);
    }
    setLoading(false);
  };
  useEffect(() => {
    if (isEdit) {
      loadToken().then();
    }
    loadAvailableModels().then();
  }, []);

  const loadAvailableModels = async () => {
    let res = await API.get(`/api/user/available_models`);
    const { success, message, data } = res.data;
    if (success) {
      let options = data.map((model) => {
        return {
          key: model,
          text: model,
          value: model
        };
      });
      setModelOptions(options);
    } else {
      showError(message);
    }
  };

  const submit = async () => {
    if (!isEdit && inputs.name === '') return;
    let localInputs = inputs;
    localInputs.remain_quota = parseInt(localInputs.remain_quota);
    localInputs.batch_number = parseInt(localInputs.batch_number);
    localInputs.rpm_limit = parseInt(localInputs.rpm_limit);
    localInputs.dpm_limit = parseInt(localInputs.dpm_limit);
    localInputs.tpm_limit = parseInt(localInputs.tpm_limit);
    localInputs.recharge_quota = parseInt(localInputs.recharge_quota);
    if (localInputs.expired_time !== -1) {
      let time = Date.parse(localInputs.expired_time);
      if (isNaN(time)) {
        showError('过期时间格式错误！');
        return;
      }
      localInputs.expired_time = Math.ceil(time / 1000);
    }
    localInputs.models = localInputs.models.join(',');
    let res;
    if (isEdit) {
      res = await API.put(`/api/token/`, { ...localInputs, id: parseInt(tokenId) });
    } else {
      res = await API.post(`/api/token/`, localInputs);
    }
    const { success, message } = res.data;
    if (success) {
      if (isEdit) {
        showSuccess('令牌更新成功！');
      } else {
        showSuccess('令牌创建成功，请在列表页面点击复制获取令牌！');
        setInputs(originInputs);
      }
    } else {
      showError(message);
    }
  };

  return (
    <>
      <Segment loading={loading}>
        <Header as='h3'>{isEdit ? '更新令牌信息' : '创建新的令牌'}</Header>
        <Form autoComplete='new-password'>
          <Form.Field>
            <Form.Input
              label='名称'
              name='name'
              placeholder={'请输入名称'}
              onChange={handleInputChange}
              value={name}
              autoComplete='new-password'
              required={!isEdit}
            />
          </Form.Field>
          <Form.Field>
            <Form.Input
              label={`额度${renderQuotaWithPrompt(remain_quota)}`}
              name='remain_quota'
              placeholder={'请输入额度'}
              onChange={handleInputChange}
              value={remain_quota}
              autoComplete='new-password'
              type='number'
              disabled={unlimited_quota}
            />
            <div style={{ lineHeight: '40px' }}>
              <Button type={'button'} onClick={() => {
                setInputs({ ...inputs, remain_quota: 5 * 500000 })
              }}>5$</Button>
              <Button type={'button'} onClick={() => {
                setInputs({ ...inputs, remain_quota: 10 * 500000 })
              }}>10$</Button>
               <Button type={'button'} onClick={() => {
                setInputs({ ...inputs, remain_quota: 15 * 500000 })
              }}>15$</Button> <Button type={'button'} onClick={() => {
                setInputs({ ...inputs, remain_quota: 20 * 500000 })
              }}>20$</Button>
              <Button type={'button'} onClick={() => {
                setInputs({ ...inputs, remain_quota: 100 * 500000 })
              }}>100$</Button>
              <Button type={'button'} onClick={() => {
                setInputs({ ...inputs, remain_quota: 120 * 500000 })
              }}>120$</Button>
              <Button type={'button'} onClick={() => {
                setUnlimitedQuota();
              }}>{unlimited_quota ? '取消无限额度' : '设为无限额度'}</Button>
            </div>
          </Form.Field>
          {isEdit ? (<Form.Field>
            <Form.Input
              label={`增加额度`}
              name='recharge_quota'
              placeholder={'请输入额度($)'}
              onChange={handleInputChange}
              value={recharge_quota}
              type='number'
            />
          </Form.Field>
          ) : <></>}
          <Form.Field>
            <Form.Input
              label='RPM'
              name='rpm_limit'
              placeholder={'请输入RPM限制'}
              onChange={handleInputChange}
              value={rpm_limit}
              type='number'
            />
          </Form.Field>
          <Form.Field>
            <Form.Input
              label='通知邮箱'
              name='email'
              placeholder={'请输入email'}
              onChange={handleInputChange}
              value={email}
            />
          </Form.Field>
          <Form.Field>
            <Form.Input
              label='DPM'
              name='dpm_limit'
              placeholder={'请输入DPM限制'}
              onChange={handleInputChange}
              value={dpm_limit}
              type='number'
            />
          </Form.Field>
          <Form.Field>
            <Form.Input
              label='TPM'
              name='tpm_limit'
              placeholder={'请输入TPM限制'}
              onChange={handleInputChange}
              value={tpm_limit}
              type='number'
            />
          </Form.Field>
          {!isEdit ? (
            <Form.Field>
              <Form.Input
                label='批量创建'
                name='batch_number'
                placeholder={'请输入需要创建的数量'}
                onChange={handleInputChange}
                value={batch_number}
                autoComplete='new-password'
                type='number'
              />
            </Form.Field>
          ) : <></>}
          <Form.Field>
            <Form.Dropdown
              label='模型范围'
              placeholder={'请选择允许使用的模型，留空则不进行限制'}
              name='models'
              fluid
              multiple
              search
              onLabelClick={(e, { value }) => {
                copy(value).then();
              }}
              selection
              onChange={handleInputChange}
              value={inputs.models}
              autoComplete='new-password'
              options={modelOptions}
            />
          </Form.Field>
          <Form.Field>
            <Form.Input
              label='IP 限制'
              name='subnet'
              placeholder={'请输入允许访问的网段，例如：192.168.0.0/24，请使用英文逗号分隔多个网段'}
              onChange={handleInputChange}
              value={inputs.subnet}
              autoComplete='new-password'
            />
          </Form.Field>
          <Form.Field>
            <Form.Input
              label='过期时间'
              name='expired_time'
              placeholder={'请输入过期时间，格式为 yyyy-MM-dd HH:mm:ss，-1 表示无限制'}
              onChange={handleInputChange}
              value={expired_time}
              autoComplete='new-password'
              type='datetime-local'
            />
          </Form.Field>
          <div style={{ lineHeight: '40px' }}>
            <Button type={'button'} onClick={() => {
              setExpiredTime(0, 0, 0, 0);
            }}>永不过期</Button>
            <Button type={'button'} onClick={() => {
              setExpiredTime(1, 0, 0, 0);
            }}>一个月后过期</Button>
            <Button type={'button'} onClick={() => {
              setExpiredTime(0, 1, 0, 0);
            }}>一天后过期</Button>
            <Button type={'button'} onClick={() => {
              setExpiredTime(0, 0, 1, 0);
            }}>一小时后过期</Button>
            <Button type={'button'} onClick={() => {
              setExpiredTime(0, 0, 0, 1);
            }}>一分钟后过期</Button>
          </div>
          <Form.Checkbox
                checked={moderations_enable}
                label='开启内容审核(针对OpenAI渠道)'
                name='moderations_enable'
                onChange={() => setModerationsEnable()}
              />
          <Form.Field>
            <Form.Input
              label='自定义联系方式'
              name='custom_contact'
              placeholder={'请输入自定义联系方式'}
              onChange={handleInputChange}
              value={custom_contact}
            />
          </Form.Field>
          <Button positive onClick={submit}>提交</Button>
          <Button onClick={handleCancel}>取消</Button>
        </Form>
      </Segment>
    </>
  );
};

export default EditToken;
