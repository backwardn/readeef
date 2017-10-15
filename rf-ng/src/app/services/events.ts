/// <reference path="./eventsource.d.ts" />

import { Injectable } from '@angular/core'
import { Observable } from "rxjs";
import { Serializable } from "./api";
import { TokenService } from './auth'

export class FeedUpdateEvent extends Serializable {
    feedID: number
    articleIDs: number[]
}

export class ArticleStateEvent extends Serializable {
    state: string
    value: boolean
    options: QueryOptions
}

export interface QueryOptions {
    ids?: number[]
    feedIDs?: number[]
    readOnly?: boolean
    unreadOnly?: boolean
    favoriteOnly?: boolean
    untaggedOnly?: boolean
    beforeID?: number
    afterID?: number
    beforeDate?: Date
    afterDate?: Date
}

@Injectable()
export class EventService {
    feedUpdate : Observable<FeedUpdateEvent>
    articleState : Observable<ArticleStateEvent>

    private eventSourceObservable : Observable<EventSource>

    constructor(private tokenService : TokenService) {
        this.eventSourceObservable = this.tokenService.tokenObservable(
        ).scan((source: EventSource, token :string) : EventSource => {
            if (source != null) {
                source.close()
            }

            if (token != "") {
                source = new EventSource("/api/v2/events?token=" + token)
            }

            return source
        }, <EventSource> null).filter(
            source => source != null
        ).shareReplay(1)

        this.feedUpdate = this.eventSourceObservable.flatMap(source => 
            Observable.fromEvent(source, "feed-update")
        ).map((event : DataEvent) =>
            new FeedUpdateEvent().fromJSON(JSON.parse(event.data))
        )

        this.articleState = this.eventSourceObservable.flatMap(source => 
            Observable.fromEvent(source, "article-state-change")
        ).map((event: DataEvent) =>
            new ArticleStateEvent().fromJSON(JSON.parse(event.data))
        )
    }
}