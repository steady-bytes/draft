import * as React from 'react';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import { AccountTree, MiscellaneousServices, House } from '@mui/icons-material';

export const mainListItems = (
  <React.Fragment>
    <ListItemButton>
      <ListItemIcon>
        <House />
      </ListItemIcon>
      <ListItemText primary="Home" />
    </ListItemButton>
    <ListItemButton>
      <ListItemIcon>
        <MiscellaneousServices />
      </ListItemIcon>
      <ListItemText primary="Services" />
    </ListItemButton>
    <ListItemButton>
      <ListItemIcon>
        <AccountTree />
      </ListItemIcon>
      <ListItemText primary="Key/Values" />
    </ListItemButton>
  </React.Fragment>
);