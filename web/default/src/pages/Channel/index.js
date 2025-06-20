import React from 'react';
import { Header, Segment, Container  } from 'semantic-ui-react';
import ChannelsTable from '../../components/ChannelsTable';

const Channel = () => (
  <>
      <Segment style={{ minWidth: 0 }}>
        <Header as='h3'>管理渠道</Header>
        <ChannelsTable />
      </Segment>
  </>
);

export default Channel;
