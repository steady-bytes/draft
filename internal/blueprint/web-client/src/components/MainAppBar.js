import * as React from 'react';
import { styled } from '@mui/material/styles';
import Toolbar from '@mui/material/Toolbar';
import Menu from '@mui/material/Menu';
import MenuIcon from '@mui/icons-material/Menu';
import NotificationsIcon from '@mui/icons-material/Notifications';
import Divider from '@mui/material/Divider';
import MenuList from '@mui/material/MenuList';
import MenuItem from '@mui/material/MenuItem';
import ListItemText from '@mui/material/ListItemText';
import ListItemIcon from '@mui/material/ListItemIcon';
import SettingsIcon from '@mui/icons-material/Settings';
import LegendToggleIcon from '@mui/icons-material/LegendToggle';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import DeblurIcon from '@mui/icons-material/Deblur';
import Typography from '@mui/material/Typography';
import IconButton from '@mui/material/IconButton';
import Badge from '@mui/material/Badge';
import MuiAppBar from '@mui/material/AppBar';

const AppBar = styled(MuiAppBar, {shouldForwardProp: (prop) => prop !== 'open', })(({ theme, open }) => ({
    zIndex: theme.zIndex.drawer + 1,
    transition: theme.transitions.create(['width', 'margin'], {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.leavingScreen,
    }),
    ...(open && {
      marginLeft: drawerWidth,
      width: `calc(100% - ${drawerWidth}px)`,
      transition: theme.transitions.create(['width', 'margin'], {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.enteringScreen,
      }),
    }),
  }));

export default function MainAppBar(props) {
    const [anchorEl, setAnchorEl] = React.useState(null);
    const open = Boolean(anchorEl);
    const handleClick = (event) => {
      setAnchorEl(event.currentTarget);
    };
    const handleClose = () => {
      setAnchorEl(null);
    };

    return (
        <AppBar position="absolute">
          <Toolbar>
            <IconButton 
              color="inherit" 
              sx={{ marginRight: '24px' }} 
              id="basic-button"
              aria-controls={open ? 'basic-menu' : undefined}
              aria-haspopup="true"
              aria-expanded={open ? 'true' : undefined}
              onClick={handleClick}
            >
              <MenuIcon />
            </IconButton>

            <Menu
              id="basic-menu"
              anchorEl={anchorEl}
              open={open}
              onClose={handleClose}
              MenuListProps={{
                'aria-labelledby': 'basic-button',
              }}
            >
              <MenuList dense sx={{width: 300, maxWidth: '100%'}}>
                <MenuItem>
                  <ListItemIcon>
                    <LegendToggleIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText>Metrics</ListItemText>
                  <Typography variant="body2" color="text.secondary">⌘M</Typography>
                </MenuItem>

                <MenuItem>
                  <ListItemIcon>
                    <AccountTreeIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText>Services</ListItemText>
                  <Typography variant="body2" color="text.secondary">⌘X</Typography>
                </MenuItem>

                <MenuItem>
                  <ListItemIcon>
                    <DeblurIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText>Key/Values</ListItemText>
                  <Typography variant="body2" color="text.secondary">⌘K</Typography>
                </MenuItem>

                <Divider />
                <MenuItem>
                  <ListItemIcon>
                    <SettingsIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText>Settings</ListItemText>
                </MenuItem>
              </MenuList>
            </Menu>

            <Typography component="h1" variant="h6" color="inherit" noWrap sx={{ flexGrow: 1 }} >
              {"{blueprint}"} 
            </Typography>

            <IconButton color="inherit">
              <Badge badgeContent={4} color="secondary">
                <NotificationsIcon />
              </Badge>
            </IconButton>
          </Toolbar>
        </AppBar>
    )
}