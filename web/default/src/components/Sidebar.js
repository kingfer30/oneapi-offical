import React, { useContext, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { UserContext } from '../context/User';
import { Icon, Menu, Button, Dropdown } from 'semantic-ui-react';
import { API, getLogo, getSystemName, isAdmin, showSuccess } from '../helpers';

// Sidebar menu items
const sidebarItems = [
  {
    name: '首页',
    to: '/',
    icon: 'home'
  },
  {
    name: '渠道',
    to: '/channel',
    icon: 'sitemap',
    admin: true
  },
  {
    name: '令牌',
    to: '/token',
    icon: 'key'
  },
  {
    name: '兑换',
    to: '/redemption',
    icon: 'dollar sign',
    admin: true
  },
  {
    name: '充值',
    to: '/topup',
    icon: 'cart'
  },
  {
    name: '用户',
    to: '/user',
    icon: 'user',
    admin: true
  },
  {
    name: '日志',
    to: '/log',
    icon: 'book'
  },
  {
    name: '设置',
    to: '/setting',
    icon: 'setting'
  },
  {
    name: '商城',
    to: '',
    icon: 'cart',
    onClick: function () {
      window.open('https://shop.aichat199.com', '_blank');
    }
  },
  {
    name: '关于',
    to: '/about',
    icon: 'info circle'
  }
];

// Add chat if enabled
if (localStorage.getItem('chat_link')) {
  sidebarItems.splice(1, 0, {
    name: '聊天',
    to: '/chat',
    icon: 'comments'
  });
}

const Sidebar = ({ collapsed, onToggleCollapse }) => {
  const [userState, userDispatch] = useContext(UserContext);
  const navigate = useNavigate();
  const location = useLocation();
  const systemName = getSystemName();
  const logo = getLogo();

  async function logout() {
    await API.get('/api/user/logout');
    showSuccess('注销成功!');
    userDispatch({ type: 'logout' });
    localStorage.removeItem('user');
    navigate('/login');
  }

  return (
    <div className={`sidebar ${collapsed ? 'collapsed' : ''}`}>
      <div className="sidebar-header">
        <div className="sidebar-header-content">
          <img src={logo} alt='logo' className="sidebar-logo" />
          {!collapsed && <h3>{systemName}</h3>}
        </div>
        <Icon
          name={collapsed ? 'angle right' : 'angle left'}
          className="collapse-toggle"
          onClick={onToggleCollapse}
        />
      </div>
      
      <Menu vertical inverted fluid className="sidebar-menu">
        {sidebarItems.map((item) => {
          if (item.admin && !isAdmin()) return null;
          
          if (item.onClick) {
            return (
              <Menu.Item
                key={item.name}
                onClick={item.onClick}
                title={collapsed ? item.name : ''}
              >
                <Icon name={item.icon} />
                {!collapsed && <span className="menu-text">{item.name}</span>}
              </Menu.Item>
            );
          }
          
          return (
            <Menu.Item
              key={item.name}
              as={Link}
              to={item.to}
              active={location.pathname === item.to}
              title={collapsed ? item.name : ''}
            >
              <Icon name={item.icon} />
              {!collapsed && <span className="menu-text">{item.name}</span>}
            </Menu.Item>
          );
        })}
      </Menu>
      
      <div className="sidebar-footer">
        {userState.user ? (
          <Menu.Item
            onClick={logout}
            title={collapsed ? '退出登录' : ''}
            className="logout-item"
          >
            <Icon name='sign out' />
            {!collapsed && <span className="menu-text">退出登录</span>}
          </Menu.Item>
        ) : (
          <div className="login-buttons">
            {collapsed ? (
              <Icon
                name='sign in'
                size='large'
                onClick={() => navigate('/login')}
                className="login-icon"
                title="登录"
              />
            ) : (
              <>
                <Button
                  primary
                  fluid
                  onClick={() => navigate('/login')}
                  style={{ marginBottom: '10px' }}
                >
                  登录
                </Button>
                <Button
                  secondary
                  fluid
                  onClick={() => navigate('/register')}
                >
                  注册
                </Button>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default Sidebar;