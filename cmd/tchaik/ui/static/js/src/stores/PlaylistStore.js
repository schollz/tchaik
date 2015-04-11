'use strict';

var AppDispatcher = require('../dispatcher/AppDispatcher');
var EventEmitter = require('eventemitter3').EventEmitter;
var assign = require('object-assign');

var CollectionStore = require('./CollectionStore.js');

var CollectionConstants = require('../constants/CollectionConstants.js');
var PlaylistConstants = require('../constants/PlaylistConstants.js');
var NowPlayingConstants = require('../constants/NowPlayingConstants.js');
var ControlApiConstants = require('../constants/ControlApiConstants.js');

var CHANGE_EVENT = 'change';

var _playlistItems = null;
var _playlistCurrent = null;

function setPlaylistItems(items) {
  _playlistItems = items;
  localStorage.setItem("playlistItems", JSON.stringify(items));
}

function getPlaylistItems() {
  if (_playlistItems === null) {
    var items = localStorage.getItem("playlistItems");
    if (items === null) {
      _playlistItems = [];
    } else {
      _playlistItems = JSON.parse(items);
    }
  }
  return _playlistItems;
}

function getPlaylistItem(itemIndex) {
  var playlistItems = getPlaylistItems();
  return playlistItems[itemIndex];
}

function setPlaylistCurrent(current) {
  _playlistCurrent = current;
  localStorage.setItem("playlistCurrent", JSON.stringify(current));
}

function getPlaylistCurrent() {
  if (_playlistCurrent !== null) {
    return _playlistCurrent;
  }
  var current = localStorage.getItem("playlistCurrent");
  if (current === null) {
    return null;
  }
  _playlistCurrent = JSON.parse(current);
  return _playlistCurrent;
}

function buildPlaylistItem(root) {
  var queue = [[]];
  var tracks = [];
  function getGroupKey(path) {
    return function(g) {
      var k = g.Key;
      queue.push(path.concat(g.Key));
      return k;
    };
  }

  function getTrackKey(path) {
    var i = 0;
    return function() {
      tracks.push(path.concat(i));
      return i++;
    };
  }

  var data = {};
  var paths = [];
  while (queue.length > 0) {
    var p = queue.shift();
    var rp = root.concat(p); // path from root
    var c = CollectionStore.getCollection(rp);

    var x = {};
    if (c.Groups) {
      x = {
        type: PlaylistConstants.TYPE_GROUP,
        keys: c.Groups.map(getGroupKey(p)),
      };
    } else if (c.Tracks) {
      x = {
        type: PlaylistConstants.TYPE_TRACKS,
        keys: c.Tracks.map(getTrackKey(p)),
      };
    } else {
      console.error("expected c.Groups or c.Tracks to be non-null");
    }
    paths.push(p);
    data[CollectionStore.pathToKey(rp)] = x;
  }

  return {
    root: root,
    paths: paths,
    data: data,
    tracks: tracks,
  };
}

function reset() {
  var items = getPlaylistItems();
  var current = null;
  if (items.length > 0) {
    current = {
      item: 0,
      track: 0,
      path: items[0].tracks[0],
    };
  }
  setPlaylistCurrent(current);
}

function next() {
  var current = getPlaylistCurrent();
  if (current === null) {
    reset();
    return;
  }

  var items = getPlaylistItems();
  var item = items[current.item];
  var tracks = item.tracks;

  if (current.track + 1 < tracks.length) {
    current.path = item.root.concat(tracks[++current.track]);
    setPlaylistCurrent(current);
    return;
  }

  if (current.item + 1 < items.length) {
    item = items[++current.item];
    current.track = 0;
    current.path = item.root.concat(item.tracks[0]);
    setPlaylistCurrent(current);
    return;
  }

  // Overflow - at the end - must be last item.
  setPlaylistCurrent(null);
}

function prev() {
  var current = getPlaylistCurrent();
  if (current === null) {
    reset();
    return;
  }

  var item = getPlaylistItem(current.item);
  var tracks = item.tracks;
  if (current.track > 0) {
    current.path = item.root.concat(tracks[--current.track]);
    setPlaylistCurrent(current);
    return;
  }

  if (current.item > 0) {
    item = getPlaylistItem(--current.item);
    current.track = item.tracks.length-1;
    current.path = item.root.concat(item.tracks[current.track]);
    setPlaylistCurrent(current);
    return;
  }
}

function canPrev() {
  var current = getPlaylistCurrent();
  if (current === null) {
    return false;
  }
  return (current.track > 0) || (current.item > 0);
}

function canNext() {
  var current = getPlaylistCurrent();
  if (current === null) {
    return false;
  }

  var items = getPlaylistItems();
  var tracks = items[current.item].tracks;
  return (current.track < (tracks.length - 1)) || (current.item < (items.length-1));
}

function getNext() {
  var current = getPlaylistCurrent();
  if (current === null) {
    return null;
  }

  var item = getPlaylistItem(current.item);
  var tracks = item.tracks;
  if (current.track < (tracks.length - 1)) {
    return trackForPath(item.root.concat(tracks[current.track+1]));
  }
  return null;
}

function remove(itemIndex, path) {
  function _isPathPrefix(path, prefix) {
    if (prefix.length > path.length) {
      return false;
    }
    for (var i = 0; i < prefix.length; i++) {
      if (path[i] != prefix[i]) {
        return false;
      }
    }
    return true;
  }

  function _pathsEqual(p1, p2) {
    return (p1.length == p2.length) && _isPathPrefix(p1, p2);
  }

  function _removeTracks(tracks, path) {
    var i = 0;
    while (i < tracks.length) {
      if (_isPathPrefix(tracks[i], path)) {
        tracks.splice(i, 1);
        continue;
      }
      i++;
    }
  }

  function _removePaths(paths, data, path) {
    var i = 0;
    while (i < paths.length) {
      if (_isPathPrefix(paths[i], path)) {
        delete data[CollectionStore.pathToKey(paths[i])];
        paths.splice(i, 1);
        continue;
      }
      i++;
    }
  }

  function _removeItem(items, itemIndex) {
    items.splice(itemIndex, 1);
  }

  var items = getPlaylistItems();
  var item = items[itemIndex];

  var data;
  do {
    if (_pathsEqual(path, item.root)) {
      _removeItem(items, itemIndex);
      break;
    }

    _removeTracks(item.tracks, path);
    _removePaths(item.paths, item.data, path);

    var last = path.pop();
    data = item.data[CollectionStore.pathToKey(path)];
    if (!data || !data.keys) {
      break;
    }
    data.keys.splice(data.keys.indexOf(last), 1);
  } while(data.keys.length === 0 && path.length > 1);

  setPlaylistCurrent(null);
  setPlaylistItems(items);
}

function setCurrent(itemIndex, path) {
  var item = getPlaylistItem(itemIndex);
  var key = CollectionStore.pathToKey(path);

  var track = -1;
  for (var i = 0; i < item.tracks.length; i++) {
    if (CollectionStore.pathToKey(item.root.concat(item.tracks[i])) === key) {
      track = i;
      break;
    }
  }
  if (track === -1) {
    console.error("Could not find track for path:"+path);
  }

  setPlaylistCurrent({
    item: itemIndex,
    path: path,
    track: track,
  });
}

function trackForPath(path) {
  var i = path.pop();
  var t = CollectionStore.getCollection(path);
  if (t === null) {
    console.log("Could not find collection with path:"+path);
  }

  if (t.Tracks) {
    var track = t.Tracks[i];
    if (track) {
      return track;
    }
    console.log("No track found for index: "+i);
    console.log(t.Tracks);
    return null;
  }

  console.log("Collection item did not have Tracks property");
  return null;
}

function currentTrack() {
  var current = getPlaylistCurrent();
  if (current === null) {
    return null;
  }
  return trackForPath(current.path.slice(0));
}

var PlaylistStore = assign({}, EventEmitter.prototype, {

  getPlaylist: function() {
    return getPlaylistItems();
  },

  getCurrent: function() {
    return getPlaylistCurrent();
  },

  getCurrentTrack: function() {
    return currentTrack();
  },

  canPrev: function() {
    return canPrev();
  },

  canNext: function() {
    return canNext();
  },

  getNext: function() {
    return getNext();
  },

  getItemKeys: function(index, path) {
    var item = getPlaylistItem(index);
    var key = CollectionStore.pathToKey(path);
    return item.data[key];
  },

  emitChange: function() {
    this.emit(CHANGE_EVENT);
  },

  /**
   * @param {function} callback
   */
  addChangeListener: function(callback) {
    this.on(CHANGE_EVENT, callback);
  },

  /**
   * @param {function} callback
   */
  removeChangeListener: function(callback) {
    this.removeListener(CHANGE_EVENT, callback);
  }

});

PlaylistStore.dispatchToken = AppDispatcher.register(function(payload) {
  var action = payload.action;
  var source = payload.source;

  if (source === 'SERVER_ACTION') {
    if (action.actionType === ControlApiConstants.CTRL) {
      switch (action.data) {
        case ControlApiConstants.NEXT:
          next();
          PlaylistStore.emitChange();
          break;

        case ControlApiConstants.PREV:
          prev();
          PlaylistStore.emitChange();
          break;
      }
    }
  }

  if (source === 'VIEW_ACTION') {
    var items;
    switch (action.actionType) {

      case NowPlayingConstants.ENDED:
        if (action.source !== "playlist") {
          break;
        }
        /* falls through */
      case PlaylistConstants.NEXT:
        next();
        PlaylistStore.emitChange();
        break;

      case PlaylistConstants.PREV:
        prev();
        PlaylistStore.emitChange();
        break;

      case PlaylistConstants.REMOVE:
        remove(action.itemIndex, action.path);
        PlaylistStore.emitChange();
        break;

      case CollectionConstants.APPEND_TO_PLAYLIST:
        items = getPlaylistItems();
        items.push(buildPlaylistItem(action.path));
        setPlaylistItems(items);
        PlaylistStore.emitChange();
        break;

      case CollectionConstants.PLAY_NOW:
        items = getPlaylistItems();
        items.unshift(buildPlaylistItem(action.path));
        setPlaylistItems(items);
        reset();
        PlaylistStore.emitChange();
        break;

      case PlaylistConstants.PLAY_ITEM:
        setCurrent(action.itemIndex, action.path);
        PlaylistStore.emitChange();
        break;

      default:
        break;
    }
  }

  return true;
});

module.exports = PlaylistStore;
