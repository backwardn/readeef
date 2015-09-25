(function(root) {
    "use strict";

    var userTTL = 1000 * 60 * 60 * 24 * 15,
        state = {VALIDATING: 1 << 0};

    function randomTheme() {
        if (document.body.classList.contains('theme-__random__')) {
            var classes = ['blue', 'indigo', 'cyan', 'teal', 'green', 'light-green', 'lime', 'red', 'pink', 'purple', 'deep-purple', 'yellow', 'amber', 'deep-orange', 'grep'],
                index = Math.floor(Math.random() * classes.length - 1);

            for (var i = 0, c; c = document.body.classList[i]; ++i) {
                if (c != "theme-__random__" && c.indexOf('theme-') == 0) {
                    document.body.classList.remove(c);
                    break;
                }
            }

            document.body.classList.add('theme-' + classes[index]);
            Polymer.updateStyles();
        }
    };
    setInterval(randomTheme, 1800000);

    Polymer({
        is: "rf-router",
        properties: {
            route: {
                type: String
            },
            user: {
                type: Object,
                readOnly: true,
                notify: true
            }
        },
        _state: 0,

        attached: function() {
            this.async(function() {
                if (!this.user && (this._state & state.VALIDATING) != state.VALIDATING) {
                    if (!MoreRouting.getRouteByName('splash').children[0].active || !location.pathname) {
                        MoreRouting.navigateTo('login');
                    } else if (!MoreRouting.isCurrentUrl('login')) {
                        MoreRouting.navigateTo('login-from', {url: this.encodeURI(location.pathname)});
                    }
                }
            });

            document.addEventListener('rf-lazy-insert', function(event) {
                Polymer.updateStyles();
            }.bind(this));
        },

        onRouteChange: function(event, detail) {
            // For some reason, MoreRouting keeps logout active for some time after redirecting
            if (MoreRouting.isCurrentUrl('logout') && !MoreRouting.isCurrentUrl('login')) {
                this.debounce('logout', function() {
                    this.logout();
                });
                return;
            }

            if (!this.user && (this._state & state.VALIDATING) != state.VALIDATING) {
                if (MoreRouting.isCurrentUrl('login')) {
                    this.$.splash.selected = 0;
                } else if (!MoreRouting.getRouteByName('splash').children[0].active || !location.pathname) {
                    MoreRouting.navigateTo('login');
                } else {
                    MoreRouting.navigateTo('login-from', {url: this.encodeURI(location.pathname)});
                }
            }

            if (MoreRouting.isCurrentUrl('login')) {
                var login = Polymer.dom(this.root).querySelector('rf-login');
                if (login) {
                    login.show();
                }
            }
        },

        onUserLoad: function(event, detail) {
            var storage = event.target;

            if (storage.value) {
                if (!storage.value.authTime || new Date().getTime() - storage.value.authTime > this.userTTL) {
                    storage.value = null;
                }
            }

            if (!detail.externalChange) {
                this.validateUser(storage.value);
            }
        },

        validateUser: function(user) {
            if (!user) {
                return;
            }

            this._state |= state.VALIDATING;

            var authCheck = this.$['auth-check'];
            var validateMessage = function(event) {
                authCheck.removeEventListener('rf-api-message', validateMessage);
                if (!event.detail.arguments.Auth) {
                    return this.connectionUnauthorized();
                }

                var user = event.detail.arguments.User;
                user.authTime = new Date().getTime();
                user.capabilities = event.detail.arguments.Capabilities;

                user.ProfileData = user.ProfileData || {};

                if (('language' in user.ProfileData) && user.ProfileData.language != this.dataset.language) {
                    location.href = location.href.replace('/' + this.dataset.language + '/', '/' + user.ProfileData.language + '/');
                }

                this._setUser(user);
                this._state &= ~state.VALIDATING;

                if (MoreRouting.getRouteByName('login-from').active) {
                    var login = Polymer.dom(this.root).querySelector('rf-login');
                    if (login) {
                        login.hide();
                    }

                    var url = MoreRouting.getRouteByName('login-from').params.url;

                    try {
                        MoreRouting.navigateTo(this.decodeURI(url));
                    } catch(e) {
                        MoreRouting.navigateTo('feed', {tagOrId: 'all'});
                    }
                } else if (MoreRouting.getRouteByName('login').active) {
                    var login = Polymer.dom(this.root).querySelector('rf-login');
                    if (login) {
                        login.hide();
                    }
                    MoreRouting.navigateTo('feed', {tagOrId: 'all'});
                } else if (!MoreRouting.isCurrentUrl('feed-base') &&
                        !MoreRouting.isCurrentUrl('settings-base')) {
                    MoreRouting.navigateTo('feed', {tagOrId: 'all'});
                }
                this.$.splash.selected = 0;

                if (user.ProfileData.theme) {
                    document.body.classList.add('theme-' + user.ProfileData.theme);
                    randomTheme();
                }

                if (user.ProfileData.shareServices) {
                    user.ProfileData.shareServices.forEach(function(name) {
                        RfShareServices.get(name).active = true;
                    });
                }
            }.bind(this);

            authCheck.user = user;
            authCheck.addEventListener('rf-api-message', validateMessage);
            authCheck.send();
        },

        logout: function() {
            this.$.logout.send();
            this._setUser(null);
            this.async(function() {
                MoreRouting.navigateTo('login');
            });
        },

        connectionUnauthorized: function() {
            this._state &= ~state.VALIDATING;
            if (MoreRouting.isCurrentUrl('feed-base') || MoreRouting.isCurrentUrl('settings-base')) {
                MoreRouting.navigateTo('login-from', {url: location.pathname});
            } else if (!MoreRouting.isCurrentUrl('login')) {
                MoreRouting.navigateTo('login');
            } else {
                var login = Polymer.dom(this.root).querySelector('rf-login');
                if (login) {
                    login.invalid = true;
                }
            }
        },

        unhandledAPIError: function(data) {
            this.$['api-error'].text = "Error: " + JSON.stringify(data.error) + ", type: " + data.errorType;
            this.$['api-error'].show();
        },

        encodeURI: function(uri) {
            return encodeURIComponent(uri).replace(/%/g, '$');
        },

        decodeURI: function(encodedURI) {
            return decodeURIComponent(encodedURI.replace(/\$/g, '%'));
        },

        _computeFeedBasePayload: function(user) {
            return {user: user};
        },

        _computeSettingsBasePayload: function(user) {
            return {user: user};
        },

    });

    root.UserBehavior = {
        validateUser: function(user) {
            Polymer.dom(document).querySelector('rf-router').validateUser(user);
        },
    };

    root.NestedRouteBehavior = {
        defaultNestedRoute: function(parentName, nestedName, nestedParams) {
            if (!MoreRouting.isCurrentUrl(nestedName)) {
                MoreRouting.navigateTo(nestedName, nestedParams || {});
            }

            MoreRouting.getRouteByName(parentName).__subscribe(function(name, value) {
                if (name == "active") {
                    // Change the route with async, otherwise once it finishes,
                    // the current one will continue and will revert it
                    this.async(function() {
                        if (value) {
                            if (!MoreRouting.isCurrentUrl(nestedName)) {
                                var route = MoreRouting.getRoute(nestedName), toNotify = {};
                                for (var key in nestedParams) {
                                    if (route.params[key] === nestedParams[key]) {
                                        toNotify[key] = nestedParams[key];
                                    }
                                }

                                MoreRouting.navigateTo(MoreRouting.urlFor(nestedName, nestedParams || {}));

                                for (var key in toNotify) {
                                    route.params.__notify(key, toNotify[key]);
                                }
                            }
                        }
                    }.bind(this));
                }
            }.bind(this));
        },

        routeParamObserver: function(routeName, param, cb) {
            var route = MoreRouting.getRoute(routeName), debouncer;

            route.params.__subscribe(function(name, value) {
                if (name == param) {
                    debouncer = Polymer.Debounce(debouncer, cb.bind(this, value));
                }
            });

            route.__subscribe(function(key, value) {
                if (key == 'active' && route.params[param] !== undefined) {
                    if (value) {
                        debouncer = Polymer.Debounce(debouncer, cb.bind(this, route.params[param]));
                    } else {
                        debouncer = Polymer.Debounce(debouncer, cb.bind(this, null, true));
                    }
                }
            });

            if (route.active) {
                debouncer = Polymer.Debounce(debouncer, cb.bind(this, route.params[param]));
            }
        },

        isActiveUrl: function(routeName) {
            var params = {};
            for (var i = 1, p; p = arguments[i]; i += 2) {
                params[arguments[i]] = arguments[i+1];
            }

            return MoreRouting.isCurrentUrl(routeName, params);
        },

    };

})(window);
