"use strict";

import AppDispatcher from "../dispatcher/AppDispatcher";
import {ChangeEmitter} from "../utils/ChangeEmitter.js";

import CollectionStore from "./CollectionStore.js";

import PlaylistConstants from "../constants/PlaylistConstants.js";


class Item {
  constructor(path, transforms) {
    this._path = path;
    this._transforms = transforms;
  }

  path() {
    return this._path;
  }

  transform(root, keys) {
    if (this._transforms === null) {
      return keys;
    }
    let result = [];
    for (let i = 0; i < keys.length; i++) {
      let k = CollectionStore.pathToKey(root.concat(keys[i]));
      let skip = false;
      for (let j = 0; j < this._transforms.length; j++) {
        if (this._transforms[j].action === "remove") {
          if (CollectionStore.pathToKey(this._transforms[j].path) === k) {
            skip = true;
            break;
          }
        }
      }
      if (!skip) {
        result.push(keys[i]);
      }
    }
    return result;
  }
}

class Playlist {
  constructor() {
    this._items = [];
    this._cursor = null;
  }

  addItem(item) {
    this._items.push(item);
  }

  clear() {
    this._items = [];
  }

  items() {
    return this._items;
  }

  cursor() {
    return this._cursor;
  }
}

const _playlist = new Playlist();

function getKeys(path) {
  let c = CollectionStore.getCollection(path);
  let keys = [];

  if (!c) {
    return keys;
  }

  if (c.tracks) {
    keys = c.tracks.map(function(k, i) {
      return i;
    });
  }

  if (c.groups) {
    keys = c.groups.map(function(g) {
      return g.key;
    });
  }
  return keys;
}


class PlaylistStore extends ChangeEmitter {

  getPlaylist() {
    return _playlist.items();
  }

  getItemKeys(index, path) {
    let keys = getKeys(path);
    const item = _playlist.items()[index];
    keys = item.transform(path, keys);
    return keys;
  }

}

const _store = new PlaylistStore();

_store.dispatchToken = AppDispatcher.register(function(payload) {
  const action = payload.action;
  const source = payload.source;

  if (source === "SERVER_ACTION") {
    if (action.actionType === PlaylistConstants.PLAYLIST) {
      _playlist.clear();
      const items = action.data.items || [];
      for (const item of items) {
        _playlist.addItem(new Item(item.path, item.transforms));
      }
      _store.emitChange();
    }
  }

  if (source === "VIEW_ACTION") {
    switch (action.actionType) {

      case PlaylistConstants.ADD_ITEM:
        // TODO: Implement
        break;

      case PlaylistConstants.ADD_ITEM_PLAY_NOW:
        // TODO: Implement
        break;

      case PlaylistConstants.CLEAR_PLAYLIST:
        // TODO: Implement
        break;

      default:
        break;
    }
  }

  return true;
});

export default _store;
