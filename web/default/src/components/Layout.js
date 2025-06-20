import React, { useState } from 'react';
import { Icon } from 'semantic-ui-react';
import Sidebar from './Sidebar';
import TopToolbar from './TopToolbar';
import { isMobile } from '../helpers';
import './Layout.css';

const Layout = ({ children }) => {
  const [sidebarVisible, setSidebarVisible] = useState(!isMobile());
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  
  const toggleSidebar = () => {
    setSidebarVisible(!sidebarVisible);
  };

  const toggleCollapse = () => {
    setSidebarCollapsed(!sidebarCollapsed);
  };

  const getSidebarWidth = () => {
    if (isMobile()) return 200;
    return sidebarCollapsed ? 60 : 200;
  };

  const getMainMargin = () => {
    if (isMobile()) return 0;
    return sidebarCollapsed ? 0 : 0;
  };

  return (
    <div className="app-layout">
      {/* Mobile menu toggle button */}
      {isMobile() && (
        <div className="mobile-header">
          <Icon
            name={sidebarVisible ? 'close' : 'bars'}
            size='large'
            onClick={toggleSidebar}
            className="menu-toggle"
          />
        </div>
      )}
      
      {/* Sidebar */}
      <div
        className={`sidebar-container ${sidebarVisible ? 'visible' : 'hidden'} ${sidebarCollapsed ? 'collapsed' : ''}`}
        style={{ width: `${getSidebarWidth()}px` }}
      >
        <Sidebar collapsed={sidebarCollapsed && !isMobile()} onToggleCollapse={toggleCollapse} />
      </div>
      
      {/* Overlay for mobile */}
      {isMobile() && sidebarVisible && (
        <div className="sidebar-overlay" onClick={toggleSidebar} />
      )}
      
      {/* Main content */}
      <div
        className={`main-container ${sidebarVisible && !isMobile() ? 'with-sidebar' : 'full-width'}`}
        style={{
          marginLeft: sidebarVisible && !isMobile() ? `${getMainMargin()}px` : '0'
        }}
      >
        <TopToolbar />
        <div className="content-wrapper">
          {children}
        </div>
      </div>
    </div>
  );
};

export default Layout;