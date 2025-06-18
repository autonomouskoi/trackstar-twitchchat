import { bus, enumName } from "/bus.js";
import * as buspb from "/pb/bus/bus_pb.js";
import * as tstc from "/m/trackstar-twitchchat/pb/twitchchat_pb.js";
import { Cfg } from './controller.js';
import { UpdatingControlPanel } from '/tk.js';

const TOPIC_REQUEST = enumName(tstc.BusTopics, tstc.BusTopics.TRACKSTAR_TWITCH_CHAT_REQUEST);

let help = document.createElement('div');
help.innerHTML = `
<section>
<p>
<em>Announce New Tracks</em> will cause each new track to be announced in chat.
Some may find this option spammy. When changed, the change takes effect immediately.
</p>
<p>
<em>Announce Template</em> specifies how the track announcement will be formatted
using placeholders for values that can be included in the message. The placeholders
begin and end with double curly braces: (<code>{{ placholder }}</code>). The
following placeholders are available:
</p>
<dl>
    <dt><code>.track_update</code></dt>
    <dd>Details about the new track.
        <dl>
            <dt><code>.deckId</code></dt>
            <dd>The deck the track was played on. Only useful on a few devices</dd>
            <dt><code>.when</code></dt>
            <dd>A numeric timestamp of when the track was played</dd>
            <dt><code>.track</code></dt>
            <dd>Data about the track played
                <dl>
                    <dt><code>.artist</code></dt>
                    <dd>The track's artist</dd>
                    <dt><code>.title</code></dt>
                    <dd>The track's title</dd>
                </dl>
            </dd>
        </dl>
    </dd>
</dl>
<p>
Assuming you just played <em>The Danger Dance</em> by <em>Men Without Hanks</em>
on <em>Deck 3</em>:
</p>

<p>
<code>
    <blockquote>{{ .track_update.track.title }} by {{ .track_update.track.artist }}</blockquote>
</code>
Would produce <code>The Danger Dance by Men Without Hanks</code>
</p>

<p>
<code>
    <blockquote>{{ .track_update.track.artist }} - {{ .track_update.track.title }} ({{ .track_update.deckId }})</blockquote>
</code>
Would produce <code>Men Without Hanks - The Danger Dance (Deck 3)</code>
</p>

<p>
The <em>Announce Current Track</em> button will cause a one time announcement in
chat of the currently playing track.
</p>
</section>

<section>
    <h2>Chat Commands</h2>

<p>
Those in the chat can type <code>!id</code> to cause the most recent track to be
announced in chat.
</p>
<p>
Users in chat can also specify a duration to announce a track that played
previously in the current session. This duration specifies time increments using
<code>h</code> for hours, <code>m</code> for minutes, and <code>s</code> for
seconds. For example, the command <code>!id 3h47m16</code> would report which
track was most recently playing 3 hours, 47 minutes, 16 seconds ago, if any.
</p>
</section>
`;

class Config extends UpdatingControlPanel<tstc.Config> {
    private _announceCheck: HTMLInputElement;
    private _templateInput: HTMLTextAreaElement;
    private _saveButton: HTMLButtonElement;

    constructor(cfg: Cfg) {
        super({ title: 'Twitch Chat Configuration', help, data: cfg });
        this.innerHTML = `
<div class="grid grid-2-col">
<label for="check-announce">Announce New Tracks</label>
<input id="check-announce" type="checkbox" />

<label>Announce Template</label>
<button id="btn-save">Save</button>
<textarea cols="60" rows="5" style="grid-colum-start: 1; grid-column-end: span 2">
</textarea>
</div>

<div>
    <button id="button-announce">Announce Current Track</button>
</div>
`;
        let announceButton = this.querySelector('#button-announce');
        announceButton.addEventListener('click', () => this.announce());
        this._announceCheck = this.querySelector('#check-announce');
        this._announceCheck.addEventListener('change', () => this.saveConfig());
        this._templateInput = this.querySelector('textarea');
        this._saveButton = this.querySelector('#btn-save');
        this._saveButton.addEventListener('click', () => this.saveConfig());

        cfg.subscribe((newCfg) => this.update(newCfg));
    }

    update(cfg: tstc.Config) {
        this._announceCheck.checked = cfg.announce;
        this._templateInput.value = cfg.template;
    }

    saveConfig() {
        let cfg = this.last.clone();

        cfg.announce = this._announceCheck.checked;
        cfg.template = this._templateInput.value;
        this.save(cfg);
    }

    announce() {
        let msg = new buspb.BusMessage();
        msg.topic = TOPIC_REQUEST;
        msg.type = tstc.MessageTypeRequest.TRACK_ANNOUNCE_REQ;
        msg.message = new tstc.TrackAnnounceRequest().toBinary();
        bus.sendWithReply(msg, (reply: buspb.BusMessage) => {
            if (reply.error) {
                throw reply.error;
            }
        });
    }
}
customElements.define('trackstar-twitchchat-config', Config, { extends: 'fieldset' });

function start(mainContainer: HTMLElement) {
    let cfg = new Cfg();

    mainContainer.appendChild(new Config(cfg));
    cfg.refresh();
}

export { start };