import React, { useState, useEffect } from 'react';
import { Icon, Button } from 'semantic-ui-react';
import './TopToolbar.css';

const TopToolbar = () => {
  const [theme, setTheme] = useState('system');

  useEffect(() => {
    // 从 localStorage 获取保存的主题设置
    const savedTheme = localStorage.getItem('theme') || 'system';
    setTheme(savedTheme);
    applyTheme(savedTheme);
  }, []);

  const applyTheme = (selectedTheme) => {
    const root = document.documentElement;
    
    if (selectedTheme === 'system') {
      // 跟随系统主题
      const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      if (systemPrefersDark) {
        root.setAttribute('data-theme', 'dark');
      } else {
        root.setAttribute('data-theme', 'light');
      }
    } else {
      root.setAttribute('data-theme', selectedTheme);
    }
  };

  const toggleTheme = () => {
    let newTheme;
    if (theme === 'system') {
      newTheme = 'light';
    } else if (theme === 'light') {
      newTheme = 'dark';
    } else {
      newTheme = 'system';
    }
    
    setTheme(newTheme);
    localStorage.setItem('theme', newTheme);
    applyTheme(newTheme);
  };

  const getThemeIcon = () => {
    switch (theme) {
      case 'light':
        return 'sun';
      case 'dark':
        return 'moon';
      default:
        return 'desktop';
    }
  };

  const getThemeText = () => {
    switch (theme) {
      case 'light':
        return '浅色';
      case 'dark':
        return '深色';
      default:
        return '跟随系统';
    }
  };

  return (
    <div className="top-toolbar">
      <div className="toolbar-right">
        <Button
          icon
          basic
          onClick={toggleTheme}
          title={`当前主题: ${getThemeText()}`}
          className="theme-toggle-btn"
        >
          <Icon name={getThemeIcon()} />
        </Button>
      </div>
    </div>
  );
};

export default TopToolbar;